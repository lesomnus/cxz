package installer

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webconfig"
	"github.com/lesomnus/cxz/internal/webui"
)

// InstallWeb shares only the Manager's run directory, never its database or Docker socket.
func InstallWeb(ctx context.Context, root string, cfg webconfig.Config, out io.Writer) error {
	lock, err := core.Lock(filepath.Join(root, "install.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	v, err := transport.Load(root)
	if err != nil {
		return fmt.Errorf("install the Manager first: %w", err)
	}
	manager, err := dockerx.Owned(ctx, v.Container, v.Owner, "")
	if err != nil {
		return err
	}
	if !manager.State.Running {
		return fmt.Errorf("start the Manager with cxz install first")
	}
	if err = replaceWeb(ctx, v, cfg, out); err != nil {
		return err
	}
	return core.WriteJSON(filepath.Join(root, "web-installation.json"), cfg)
}

// RefreshWebLocked is called under install.lock after a Manager/version update.
// A deliberately stopped or removed web container stays stopped/removed.
func RefreshWebLocked(ctx context.Context, root string, out io.Writer) error {
	b, err := os.ReadFile(filepath.Join(root, "web-installation.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg webconfig.Config
	if err = json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	v, err := transport.Load(root)
	if err != nil {
		return err
	}
	old, exists, err := findWeb(ctx, v)
	if err != nil || !exists {
		return err
	}
	if !old.State.Running {
		return nil
	}
	// No-op after a successful retry; changing configuration is explicit web up.
	if old.Config.Image == v.Image {
		return nil
	}
	if err = replaceWeb(ctx, v, cfg, out); err != nil {
		return fmt.Errorf("Manager updated but web update failed; retry cxz web up: %w", err)
	}
	return nil
}

func findWeb(ctx context.Context, v transport.Installation) (dockerx.Container, bool, error) {
	cs, err := dockerx.List(ctx, "name=^/"+v.Container+"-web$")
	if err != nil {
		return dockerx.Container{}, false, err
	}
	if len(cs) == 0 {
		return dockerx.Container{}, false, nil
	}
	if len(cs) != 1 || cs[0].Config.Labels["cxz.owner"] != v.Owner || cs[0].Config.Labels["cxz.role"] != "web" {
		return dockerx.Container{}, false, fmt.Errorf("web container name is occupied by an unowned container")
	}
	return cs[0], true, nil
}

func webArgs(v transport.Installation, cfg webconfig.Config) ([]string, error) {
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("web listen: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("web listen must have a fixed port from 1 to 65535")
	}
	if host != "" && net.ParseIP(host) == nil {
		return nil, fmt.Errorf("installed web listen host must be an IP address")
	}
	// The gateway binds every interface of its own network namespace, so this
	// publish address is the only thing deciding whether it can be reached from
	// a network -- and therefore the only place a plaintext origin can be gated.
	if cfg.Certificate == "" && !webui.Loopback(host) {
		return nil, fmt.Errorf("a plaintext gateway must publish on a loopback address, not %q; configure an https origin with tls_cert and tls_key to serve a network", cfg.Listen)
	}
	publish := port + ":7350"
	if host != "" {
		publish = net.JoinHostPort(host, port) + ":7350"
	}
	// Host-owned 0600 keys need DAC_OVERRIDE for container root. All mounts and
	// the root filesystem remain read-only; no other capabilities are granted.
	args := []string{"run", "-d", "--name", v.Container + "-web", "--restart", "unless-stopped", "--label", "cxz.role=web", "--label", "cxz.owner=" + v.Owner, "--read-only", "--cap-drop=ALL", "--cap-add=DAC_OVERRIDE", "--security-opt=no-new-privileges", "--publish", publish, "--mount", "type=volume,source=" + v.StateVolume + ",target=/var/lib/cxz/run,volume-subpath=run,readonly"}
	mounts := [][2]string{{cfg.TokenFile, "token"}}
	if cfg.Certificate != "" {
		mounts = append(mounts, [2]string{cfg.Certificate, "certificate.pem"}, [2]string{cfg.Key, "key.pem"})
	}
	for _, pair := range mounts {
		args = append(args, "--mount", "type=bind,source="+pair[0]+",target=/web/"+pair[1]+",readonly")
	}
	args = append(args, v.Image, "--state", "/var/lib/cxz", "-x", "_web-serve", "--listen", "0.0.0.0:7350", "--origin", cfg.Origin, "--access-token-file", "/web/token")
	if cfg.Certificate != "" {
		args = append(args, "--tls-cert", "/web/certificate.pem", "--tls-key", "/web/key.pem")
	}
	return args, nil
}

func replaceWeb(ctx context.Context, v transport.Installation, cfg webconfig.Config, out io.Writer) error {
	if _, err := cfg.Runtime(); err != nil {
		return err
	}
	args, err := webArgs(v, cfg)
	if err != nil {
		return err
	}
	old, exists, err := findWeb(ctx, v)
	if err != nil {
		return err
	}
	// Validate the target image and command before disrupting a working gateway.
	if _, err = dockerx.Run(ctx, "run", "--rm", "--entrypoint", "/usr/local/bin/cxz", v.Image, "_web-serve", "--help"); err != nil {
		return fmt.Errorf("target image does not support web: %w", err)
	}
	name := v.Container + "-web"
	backup := name + "-previous-" + core.ID()[:12]
	if exists {
		if _, err = dockerx.Run(ctx, "rename", old.ID, backup); err != nil {
			return err
		}
		if _, err = dockerx.Run(ctx, "stop", "--time", "10", old.ID); err != nil {
			_, _ = dockerx.Run(context.WithoutCancel(ctx), "rename", old.ID, name)
			return err
		}
	}
	created := false
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if created {
			if _, e := dockerx.Run(recovery, "rm", "-f", name); e != nil {
				return fmt.Errorf("%w; rollback removal failed: %v (previous container %s)", cause, e, backup)
			}
		}
		if exists {
			if _, e := dockerx.Run(recovery, "rename", old.ID, name); e != nil {
				return fmt.Errorf("%w; rollback rename failed: %v", cause, e)
			}
			if old.State.Running {
				if _, e := dockerx.Run(recovery, "start", old.ID); e != nil {
					return fmt.Errorf("%w; rollback start failed: %v", cause, e)
				}
			}
		}
		return cause
	}
	if _, err = dockerx.Run(ctx, args...); err != nil {
		// docker run can create a container before failing to start it.
		c, found, e := findWeb(ctx, v)
		if e == nil && found {
			created = c.ID != old.ID
		}
		return rollback(err)
	}
	created = true
	origin, _ := url.Parse(cfg.Origin)
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		// Probe through the container itself, on the scheme it serves. This is a
		// startup check; browsers still verify a configured certificate normally.
		_, err = dockerx.Run(probeCtx, "exec", name, "curl", "--silent", "--show-error", "--fail", "--insecure", "--max-time", "2", "--header", "Host: "+origin.Host, origin.Scheme+"://127.0.0.1:7350/")
		if err == nil {
			break
		}
		select {
		case <-probeCtx.Done():
			logs, _ := dockerx.Run(ctx, "logs", "--tail", "15", name)
			return rollback(fmt.Errorf("web did not become ready: %w; logs: %.2000s", err, logs))
		case <-time.After(200 * time.Millisecond):
		}
	}
	if exists {
		if _, err = dockerx.Run(ctx, "rm", old.ID); err != nil {
			return fmt.Errorf("web updated; remove retained container %s: %w", backup, err)
		}
	}
	fmt.Fprintf(out, "cxz web up: %s · %s\n", name, cfg.Origin)
	return nil
}

// WebState is everything the host can say about the gateway without asking a
// browser: what it would serve, what is actually running, and whether the two
// still agree with the Manager it forwards to.
type WebState struct {
	ConfigPath        string `json:"config_path,omitempty"`
	Origin            string `json:"origin,omitempty"`
	Listen            string `json:"published,omitempty"`
	TLS               bool   `json:"tls"`
	TokenFile         string `json:"token_file,omitempty"`
	TokenPresent      bool   `json:"token_present"`
	CertificateExpiry string `json:"certificate_expires,omitempty"`
	Container         string `json:"container,omitempty"`
	State             string `json:"state"`
	Image             string `json:"image,omitempty"`
	ManagerImage      string `json:"manager_image,omitempty"`
	NeedsRefresh      bool   `json:"needs_refresh"`
}

// WebStatus reports rather than fails: a host with no Manager, no configuration
// or no container is a state to read, not an error to recover from. Only a
// question that cannot be answered -- a broken Docker, an unowned container of
// the same name -- is returned as one.
func WebStatus(ctx context.Context, root string, cfg webconfig.Config, configPath string) (WebState, error) {
	s := WebState{ConfigPath: configPath, Origin: cfg.Origin, Listen: cfg.Listen, TLS: cfg.Certificate != "", TokenFile: cfg.TokenFile, State: "not installed"}
	if cfg.TokenFile != "" {
		_, err := os.Stat(cfg.TokenFile)
		s.TokenPresent = err == nil
	}
	if cfg.Certificate != "" {
		if leaf, err := readLeaf(cfg.Certificate); err == nil {
			s.CertificateExpiry = leaf.NotAfter.UTC().Format(time.RFC3339)
		}
	}
	v, err := transport.Load(root)
	if err != nil {
		s.State = "Manager is not installed"
		return s, nil
	}
	s.Container, s.ManagerImage = v.Container+"-web", v.Image
	c, exists, err := findWeb(ctx, v)
	if err != nil {
		return s, err
	}
	if !exists {
		return s, nil
	}
	s.State, s.Image = c.State.Status, c.Config.Image
	// A gateway left behind by a version switch serves last version's UI against
	// this version's Manager, which is the one skew a reader cannot see.
	s.NeedsRefresh = c.State.Running && c.Config.Image != v.Image
	return s, nil
}

func readLeaf(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for block, rest := pem.Decode(b); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
	return nil, fmt.Errorf("no certificate in %s", path)
}

func UninstallWeb(ctx context.Context, root string) error {
	lock, err := core.Lock(filepath.Join(root, "install.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	v, err := transport.Load(root)
	if err != nil {
		return err
	}
	return removeWeb(ctx, v)
}

func removeWeb(ctx context.Context, v transport.Installation) error {
	c, exists, err := findWeb(ctx, v)
	if err != nil {
		return err
	}
	if exists {
		if _, err = dockerx.Run(ctx, "rm", "-f", c.ID); err != nil {
			return err
		}
	}
	return nil
}
