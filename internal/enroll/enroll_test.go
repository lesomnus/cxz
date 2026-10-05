package enroll

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/pki"
)

// host is the installation on the other end of the channel: the same commands
// the real `cxz expose` subcommands run, against a real authority.
type host struct {
	authority *pki.Authority
	state     string
	calls     []string
	failUp    bool
}

func newHost(t *testing.T) *host {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pki")
	a, err := pki.EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &host{authority: a, state: "0.0.0.0:7349"}
}

func (h *host) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	h.calls = append(h.calls, strings.Join(args, " "))
	switch {
	case len(args) >= 2 && args[0] == "expose" && args[1] == "ca":
		return h.authority.CertificatePEM(), nil
	case len(args) >= 2 && args[0] == "expose" && args[1] == "sign":
		label := ""
		for i, a := range args {
			if a == "--label" && i+1 < len(args) {
				label = args[i+1]
			}
		}
		cert, _, err := h.authority.SignClient(stdin, label, 0)
		return cert, err
	case len(args) >= 2 && args[0] == "expose" && args[1] == "status":
		state := "running"
		if h.state == "" {
			state = "not installed"
		}
		return json.Marshal(map[string]any{"listen": h.state, "state": state})
	case len(args) >= 2 && args[0] == "expose" && args[1] == "up":
		if h.failUp {
			return nil, fmt.Errorf("docker is not running")
		}
		h.state = "0.0.0.0:7349"
		return []byte("mtls://0.0.0.0:7349\n"), nil
	}
	return nil, fmt.Errorf("unexpected remote command %v", args)
}

// The automatic path produces something usable in one step, and the key it is
// built on never appears in anything the runner was asked to carry.
func TestEnrollOverAChannelKeepsTheKeyLocal(t *testing.T) {
	state := t.TempDir()
	h := newHost(t)
	var log bytes.Buffer
	carried := &recorder{inner: h.run}
	stored, err := Over(context.Background(), carried.run, state, "work", "build-host", Options{Label: "laptop", Out: &log})
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Usable() {
		t.Fatalf("%+v", stored)
	}
	// The port is the one the host reported; the host part is the one this
	// client reaches it by, not the wildcard the relay publishes on.
	if stored.Address != "build-host:7349" {
		t.Fatal("wrong dial address:", stored.Address)
	}
	if !strings.Contains(log.String(), "expires") || !strings.Contains(log.String(), "laptop") {
		t.Fatal("enrolling was silent:", log.String())
	}
	key, err := os.ReadFile(filepath.Join(Dir(state, "work"), "client.key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(carried.sent, key) {
		t.Fatal("the private key was sent over the channel")
	}
	if !bytes.Contains(carried.sent, []byte("CERTIFICATE REQUEST")) {
		t.Fatal("no certificate request was sent:", string(carried.sent))
	}
	for _, name := range []string{"client.key", "client.crt", "ca.crt", "address"} {
		info, err := os.Stat(filepath.Join(Dir(state, "work"), name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s is %v", name, info.Mode().Perm())
		}
	}
	loaded, ok, err := Load(state, "work")
	if err != nil || !ok || loaded.Serial != stored.Serial {
		t.Fatal(loaded, ok, err)
	}
}

// A stopped relay is the ordinary case after a host reboot. Enrolling starts it
// when allowed, and says so rather than quietly reaching further than asked.
func TestEnrollStartsAStoppedRelayOnlyWhenAsked(t *testing.T) {
	h := newHost(t)
	h.state = ""
	if _, err := Over(context.Background(), h.run, t.TempDir(), "work", "build-host", Options{}); err == nil {
		t.Fatal("a stopped relay was started without being asked")
	}
	for _, call := range h.calls {
		if strings.Contains(call, "expose up") {
			t.Fatal("the relay was started anyway:", h.calls)
		}
	}
	var log bytes.Buffer
	stored, err := Over(context.Background(), h.run, t.TempDir(), "work", "build-host", Options{Start: true, Out: &log})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Address != "build-host:7349" {
		t.Fatal(stored.Address)
	}
	if !strings.Contains(log.String(), "starting it") {
		t.Fatal("starting the relay was silent:", log.String())
	}
}

// A failure halfway must not leave a connection holding half an identity.
func TestFailedEnrollKeepsThePreviousIdentity(t *testing.T) {
	state := t.TempDir()
	h := newHost(t)
	first, err := Over(context.Background(), h.run, state, "work", "build-host", Options{Label: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	h.state, h.failUp = "", true
	if _, err = Over(context.Background(), h.run, state, "work", "build-host", Options{Start: true}); err == nil {
		t.Fatal("a failed start reported success")
	}
	again, ok, err := Load(state, "work")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if again.Serial != first.Serial || !again.Usable() {
		t.Fatal("the working identity was replaced by a failed enrollment")
	}
}

// The manual path: a request out, a certificate back, and the key stays put.
func TestManualRequestAndCompletion(t *testing.T) {
	state := t.TempDir()
	h := newHost(t)
	csr, err := Request(state, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csr), "CERTIFICATE REQUEST") {
		t.Fatal(string(csr))
	}
	if _, ok, _ := Load(state, "lab"); ok {
		t.Fatal("a pending request counts as an identity")
	}
	cert, _, err := h.authority.SignClient(csr, "lab-desktop", 0)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := Complete(state, "lab", cert, h.authority.CertificatePEM(), "10.0.0.5:7349")
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Usable() || stored.Address != "10.0.0.5:7349" {
		t.Fatalf("%+v", stored)
	}
	// Completing without a request is a mistake worth naming: the certificate
	// would have no key to go with it.
	if _, err = Complete(state, "unrequested", cert, h.authority.CertificatePEM(), "10.0.0.5:7349"); err == nil {
		t.Fatal("completed an enrollment that was never requested")
	}
	if _, err = Complete(state, "lab", []byte("not a certificate"), h.authority.CertificatePEM(), "10.0.0.5:7349"); err == nil {
		t.Fatal("accepted something that is not a certificate")
	}
	if err = Forget(state, "lab"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Load(state, "lab"); ok {
		t.Fatal("forgetting left the identity behind")
	}
	if err = Forget(state, "lab"); err == nil {
		t.Fatal("forgetting nothing reported success")
	}
}

func TestExpiredIdentityIsNotUsable(t *testing.T) {
	state := t.TempDir()
	h := newHost(t)
	csr, err := Request(state, "old")
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := h.authority.SignClient(csr, "old", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := Complete(state, "old", cert, h.authority.CertificatePEM(), "10.0.0.5:7349")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Usable() {
		t.Fatal("an expired certificate is treated as usable")
	}
	loaded, ok, err := Load(state, "old")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if loaded.Usable() {
		t.Fatal("an expired certificate loads as usable")
	}
}

type recorder struct {
	inner Runner
	sent  []byte
}

func (r *recorder) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	r.sent = append(r.sent, stdin...)
	r.sent = append(r.sent, []byte(strings.Join(args, " "))...)
	return r.inner(ctx, stdin, args...)
}
