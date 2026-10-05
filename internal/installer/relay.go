package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

// The relay puts the manager's socket on a network for cxz's own clients. It is
// a sibling of the manager rather than part of it, for the same reason the web
// gateway is: exposure is a thing you turn on and off without touching the
// manager, and the process that listens holds no more than it must.
//
// What it must hold is a certificate signed by the installation root -- never
// the root itself. The root stays in the manager's state, and only the leaf and
// the root's public certificate are mounted here, read-only.

const relayPort = 7349

type RelayConfig struct {
	Listen string `json:"listen"`
}

func relayName(v transport.Installation) string { return v.Container + "-remote" }

func InstallRelay(ctx context.Context, root string, cfg RelayConfig, out io.Writer) error {
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
	if err = replaceRelay(ctx, v, cfg, out); err != nil {
		return err
	}
	return core.WriteJSON(filepath.Join(root, "relay-installation.json"), cfg)
}

// RefreshRelayLocked runs under install.lock after a version switch, so the
// relay serves the same build as the manager it forwards to. A relay that was
// deliberately stopped or removed stays that way.
func RefreshRelayLocked(ctx context.Context, root string, out io.Writer) error {
	b, err := os.ReadFile(filepath.Join(root, "relay-installation.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg RelayConfig
	if err = json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	v, err := transport.Load(root)
	if err != nil {
		return err
	}
	old, exists, err := findRelay(ctx, v)
	if err != nil || !exists || !old.State.Running || old.Config.Image == v.Image {
		return err
	}
	if err = replaceRelay(ctx, v, cfg, out); err != nil {
		return fmt.Errorf("Manager updated but relay update failed; retry cxz expose up: %w", err)
	}
	return nil
}

func findRelay(ctx context.Context, v transport.Installation) (dockerx.Container, bool, error) {
	cs, err := dockerx.List(ctx, "name=^/"+relayName(v)+"$")
	if err != nil {
		return dockerx.Container{}, false, err
	}
	if len(cs) == 0 {
		return dockerx.Container{}, false, nil
	}
	if len(cs) != 1 || cs[0].Config.Labels["cxz.owner"] != v.Owner || cs[0].Config.Labels["cxz.role"] != "remote" {
		return dockerx.Container{}, false, fmt.Errorf("relay container name is occupied by an unowned container")
	}
	return cs[0], true, nil
}

func relayArgs(v transport.Installation, cfg RelayConfig) ([]string, error) {
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("relay listen: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("relay listen must have a fixed port from 1 to 65535")
	}
	if host != "" && net.ParseIP(host) == nil {
		return nil, fmt.Errorf("relay listen host must be an IP address")
	}
	publish := port + ":" + strconv.Itoa(relayPort)
	if host != "" {
		publish = net.JoinHostPort(host, port) + ":" + strconv.Itoa(relayPort)
	}
	// Two subpaths of the manager's state volume, both read-only: the socket it
	// forwards to, and the relay's own certificate beside the root's public one.
	// The root's private key is in neither, so this container cannot issue
	// anything -- it can only present what it was given and check what it is
	// shown.
	args := []string{"run", "-d", "--name", relayName(v), "--restart", "unless-stopped", "--label", "cxz.role=remote", "--label", "cxz.owner=" + v.Owner,
		"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--publish", publish,
		"--mount", "type=volume,source=" + v.StateVolume + ",target=/var/lib/cxz/run,volume-subpath=run,readonly",
		"--mount", "type=volume,source=" + v.StateVolume + ",target=/var/lib/cxz/pki/server,volume-subpath=pki/server,readonly",
		v.Image, "--state", "/var/lib/cxz", "-x", "_expose-serve", "--listen", "0.0.0.0:" + strconv.Itoa(relayPort)}
	return args, nil
}

func replaceRelay(ctx context.Context, v transport.Installation, cfg RelayConfig, out io.Writer) error {
	args, err := relayArgs(v, cfg)
	if err != nil {
		return err
	}
	// The manager is the only process with the root's private key, so it is the
	// one that creates it and issues the relay's certificate.
	if _, err = dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_pki", "server"); err != nil {
		return fmt.Errorf("prepare the installation root: %w", err)
	}
	old, exists, err := findRelay(ctx, v)
	if err != nil {
		return err
	}
	if _, err = dockerx.Run(ctx, "run", "--rm", "--entrypoint", "/usr/local/bin/cxz", v.Image, "_expose-serve", "--help"); err != nil {
		return fmt.Errorf("target image does not support the relay: %w", err)
	}
	name := relayName(v)
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
		if c, found, e := findRelay(ctx, v); e == nil && found {
			created = c.ID != old.ID
		}
		return rollback(err)
	}
	created = true
	// Readiness is asked inside the container: the published port belongs to the
	// Docker engine's host, which is not necessarily the machine running this
	// command, and the handshake itself would need a client certificate that
	// neither of them has a reason to hold.
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, err = dockerx.Run(ctx, "exec", name, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_expose-check", "--address", "127.0.0.1:"+strconv.Itoa(relayPort))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := dockerx.Run(ctx, "logs", "--tail", "15", name)
			return rollback(fmt.Errorf("relay did not accept connections: %w; logs: %.2000s", err, logs))
		}
		time.Sleep(200 * time.Millisecond)
	}
	if exists {
		if _, err = dockerx.Run(ctx, "rm", old.ID); err != nil {
			return fmt.Errorf("relay updated; remove retained container %s: %w", backup, err)
		}
	}
	fmt.Fprintf(out, "cxz expose up: %s · mtls://%s\n", name, cfg.Listen)
	return nil
}

func UninstallRelay(ctx context.Context, root string) error {
	lock, err := core.Lock(filepath.Join(root, "install.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	v, err := transport.Load(root)
	if err != nil {
		return err
	}
	c, exists, err := findRelay(ctx, v)
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

// RelayState is what the host can report without holding a client certificate.
type RelayState struct {
	ConfigPath   string `json:"config_path,omitempty"`
	Listen       string `json:"listen,omitempty"`
	Container    string `json:"container,omitempty"`
	State        string `json:"state"`
	Image        string `json:"image,omitempty"`
	ManagerImage string `json:"manager_image,omitempty"`
	NeedsRefresh bool   `json:"needs_refresh"`
	Root         string `json:"root_fingerprint,omitempty"`
	Clients      []any  `json:"clients,omitempty"`
}

func RelayStatus(ctx context.Context, root string) (RelayState, error) {
	s := RelayState{State: "not installed"}
	if b, err := os.ReadFile(filepath.Join(root, "relay-installation.json")); err == nil {
		var cfg RelayConfig
		if err = json.Unmarshal(b, &cfg); err == nil {
			s.ConfigPath, s.Listen = filepath.Join(root, "relay-installation.json"), cfg.Listen
		}
	}
	v, err := transport.Load(root)
	if err != nil {
		s.State = "Manager is not installed"
		return s, nil
	}
	s.Container, s.ManagerImage = relayName(v), v.Image
	if issued, err := dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_pki", "issued"); err == nil {
		var report struct {
			Root   string `json:"root"`
			Issued []any  `json:"issued"`
		}
		if json.Unmarshal(issued, &report) == nil {
			s.Root, s.Clients = report.Root, report.Issued
		}
	}
	c, exists, err := findRelay(ctx, v)
	if err != nil {
		return s, err
	}
	if !exists {
		return s, nil
	}
	s.State, s.Image = c.State.Status, c.Config.Image
	s.NeedsRefresh = c.State.Running && c.Config.Image != v.Image
	return s, nil
}
