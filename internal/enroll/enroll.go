// Package enroll is the client's half of mutual TLS: it keeps this machine's
// certificate for one named connection, and obtains one over a channel that is
// already authenticated.
//
// The key is generated here and stays here. What travels is a certificate
// request, which is public, and a certificate, which is public -- so the same
// code serves the automatic path over SSH and the manual path where a person
// moves two files themselves.
package enroll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/pki"
	"github.com/lesomnus/cxz/internal/transport"
)

// Dir holds one connection's identity. Per connection rather than per client,
// because each installation has its own root: a certificate from one is
// meaningless to another.
func Dir(state, name string) string { return filepath.Join(state, "connections", name) }

type Stored struct {
	Identity transport.Identity
	Address  string
	Expires  time.Time
	Serial   string
}

func (s Stored) Usable() bool {
	return s.Identity.Complete() && s.Address != "" && time.Now().Before(s.Expires)
}

// Load reports what this client holds for a connection. A missing identity is
// not an error: it is the normal state before enrolling, and the caller decides
// whether to enroll, fall back, or explain.
func Load(state, name string) (Stored, bool, error) {
	dir := Dir(state, name)
	cert, err := os.ReadFile(filepath.Join(dir, "client.crt"))
	if errors.Is(err, fs.ErrNotExist) {
		return Stored{}, false, nil
	}
	if err != nil {
		return Stored{}, false, err
	}
	key, err := os.ReadFile(filepath.Join(dir, "client.key"))
	if err != nil {
		return Stored{}, false, err
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return Stored{}, false, err
	}
	address, err := os.ReadFile(filepath.Join(dir, "address"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Stored{}, false, err
	}
	expiry, serial, err := pki.Expiry(cert)
	if err != nil {
		return Stored{}, false, err
	}
	return Stored{Identity: transport.Identity{Certificate: cert, Key: key, CA: ca}, Address: strings.TrimSpace(string(address)), Expires: expiry, Serial: serial}, true, nil
}

func store(state, name string, key, cert, ca []byte, address string) error {
	dir := Dir(state, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for file, content := range map[string][]byte{"client.key": key, "client.crt": cert, "ca.crt": ca, "address": []byte(address + "\n")} {
		if content == nil {
			continue
		}
		if err := core.WriteFile(filepath.Join(dir, file), content); err != nil {
			return err
		}
	}
	return nil
}

func Forget(state, name string) error {
	dir := Dir(state, name)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no identity is stored for connection %s", name)
	}
	return os.RemoveAll(dir)
}

// Request writes a new key for this connection and returns the request to be
// signed. The key replaces any previous one, so a request that is never signed
// leaves the connection without a usable identity rather than a stale one.
func Request(state, name string) ([]byte, error) {
	key, csr, err := pki.NewRequest()
	if err != nil {
		return nil, err
	}
	if err = store(state, name, key, nil, nil, ""); err != nil {
		return nil, err
	}
	return csr, nil
}

// Complete finishes the manual path: the certificate someone carried back,
// and the root to pin, joined to the key that never left.
func Complete(state, name string, cert, ca []byte, address string) (Stored, error) {
	if _, err := os.Stat(filepath.Join(Dir(state, name), "client.key")); err != nil {
		return Stored{}, fmt.Errorf("no pending request for %s; run cxz connection enroll --csr first", name)
	}
	if _, _, err := pki.Expiry(cert); err != nil {
		return Stored{}, err
	}
	if err := store(state, name, nil, cert, ca, address); err != nil {
		return Stored{}, err
	}
	stored, _, err := Load(state, name)
	return stored, err
}

// Runner runs one cxz command on the host that owns the installation, with
// stdin and stdout wired through. Over SSH that is an ssh invocation; in a test
// it is a function. Enrollment is defined in terms of it so that the trust
// anchor -- whatever authenticates the channel -- is the caller's choice.
type Runner func(ctx context.Context, stdin []byte, args ...string) ([]byte, error)

type Options struct {
	Label   string // recorded by the installation; defaults to this host's name
	Address string // override the address to dial; otherwise read from the relay
	Start   bool   // start the relay when it is not running
	Out     io.Writer
}

type remoteStatus struct {
	Listen       string `json:"listen"`
	State        string `json:"state"`
	NeedsRefresh bool   `json:"needs_refresh"`
}

// Over enrolls this client through run. The order matters: the root is fetched
// and the certificate signed before anything is stored, so a failure halfway
// leaves the previous identity in place.
func Over(ctx context.Context, run Runner, state, name, sshHost string, o Options) (Stored, error) {
	label := o.Label
	if label == "" {
		label = hostLabel()
	}
	ca, err := run(ctx, nil, "expose", "ca")
	if err != nil {
		return Stored{}, fmt.Errorf("read the installation root: %w", err)
	}
	key, csr, err := pki.NewRequest()
	if err != nil {
		return Stored{}, err
	}
	cert, err := run(ctx, csr, "expose", "sign", "--label", label)
	if err != nil {
		return Stored{}, fmt.Errorf("sign this client: %w", err)
	}
	address := o.Address
	if address == "" {
		address, err = relayAddress(ctx, run, sshHost, o)
		if err != nil {
			return Stored{}, err
		}
	}
	if err = store(state, name, key, cert, ca, address); err != nil {
		return Stored{}, err
	}
	stored, _, err := Load(state, name)
	if err != nil {
		return Stored{}, err
	}
	if o.Out != nil {
		fmt.Fprintf(o.Out, "%s: enrolled as %q with the installation root; certificate %s expires %s\n", name, label, stored.Serial, stored.Expires.UTC().Format("2006-01-02"))
	}
	return stored, nil
}

// relayAddress asks the installation where it listens rather than assuming a
// port, and starts the relay when asked to. The host part comes from the
// channel we are already talking over: the relay reports the address it is
// published on, which is frequently a wildcard that says nothing about how this
// client should reach it.
func relayAddress(ctx context.Context, run Runner, sshHost string, o Options) (string, error) {
	status, err := relay(ctx, run)
	if err != nil {
		return "", err
	}
	if status.State != "running" {
		if !o.Start {
			return "", fmt.Errorf("the relay is %s on that host; run cxz expose up there, or enroll with --start", status.State)
		}
		if o.Out != nil {
			fmt.Fprintf(o.Out, "the relay was %s; starting it over this connection\n", status.State)
		}
		if _, err = run(ctx, nil, "expose", "up"); err != nil {
			return "", fmt.Errorf("start the relay: %w", err)
		}
		if status, err = relay(ctx, run); err != nil {
			return "", err
		}
	}
	if status.Listen == "" {
		return "", fmt.Errorf("the relay did not report a listen address")
	}
	_, port, err := net.SplitHostPort(status.Listen)
	if err != nil {
		return "", fmt.Errorf("the relay reported an unusable listen address %q", status.Listen)
	}
	if sshHost == "" {
		return status.Listen, nil
	}
	return net.JoinHostPort(sshHost, port), nil
}

func relay(ctx context.Context, run Runner) (remoteStatus, error) {
	var status remoteStatus
	b, err := run(ctx, nil, "expose", "status", "--format", "json")
	if err != nil {
		return status, fmt.Errorf("read the relay's state: %w", err)
	}
	if err = json.Unmarshal(b, &status); err != nil {
		return status, fmt.Errorf("the host did not report a relay state: %w", err)
	}
	return status, nil
}

// Start brings up a relay that stopped, for a client that is already enrolled.
// It is the recovery that keeps an enrolled connection working after a host
// reboot that left the relay removed.
func Start(ctx context.Context, run Runner, out io.Writer) error {
	status, err := relay(ctx, run)
	if err != nil {
		return err
	}
	if status.State == "running" {
		return nil
	}
	if out != nil {
		fmt.Fprintf(out, "the relay was %s; starting it over ssh\n", status.State)
	}
	_, err = run(ctx, nil, "expose", "up")
	return err
}

func hostLabel() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "client"
	}
	clean := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return "client"
	}
	if len(clean) > 64 {
		clean = clean[:64]
	}
	return string(clean)
}
