package installer

import (
	"context"
	"encoding/json"
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
	// No-op after a successful retry; changing configuration is explicit install web.
	if old.Config.Image == v.Image {
		return nil
	}
	if err = replaceWeb(ctx, v, cfg, out); err != nil {
		return fmt.Errorf("Manager updated but web update failed; retry cxz install web: %w", err)
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
	publish := port + ":7350"
	if host != "" {
		publish = net.JoinHostPort(host, port) + ":7350"
	}
	args := []string{"run", "-d", "--name", v.Container + "-web", "--restart", "unless-stopped", "--label", "cxz.role=web", "--label", "cxz.owner=" + v.Owner, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--publish", publish, "--mount", "type=volume,source=" + v.StateVolume + ",target=/var/lib/cxz/run,volume-subpath=run,readonly"}
	for _, pair := range [][2]string{{cfg.Certificate, "certificate.pem"}, {cfg.Key, "key.pem"}, {cfg.TokenFile, "token"}} {
		args = append(args, "--mount", "type=bind,source="+pair[0]+",target=/web/"+pair[1]+",readonly")
	}
	args = append(args, v.Image, "--state", "/var/lib/cxz", "-x", "web", "--listen", "0.0.0.0:7350", "--origin", cfg.Origin, "--tls-cert", "/web/certificate.pem", "--tls-key", "/web/key.pem", "--access-token-file", "/web/token")
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
	if _, err = dockerx.Run(ctx, "run", "--rm", "--entrypoint", "/usr/local/bin/cxz", v.Image, "web", "--help"); err != nil {
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
		// Probe HTTPS through the container itself. This is a startup check; browsers
		// still verify the configured certificate normally.
		_, err = dockerx.Run(probeCtx, "exec", name, "curl", "--silent", "--show-error", "--fail", "--insecure", "--max-time", "2", "--header", "Host: "+origin.Host, "https://127.0.0.1:7350/")
		if err == nil {
			break
		}
		select {
		case <-probeCtx.Done():
			return rollback(fmt.Errorf("web did not become ready: %w", err))
		case <-time.After(200 * time.Millisecond):
		}
	}
	if exists {
		if _, err = dockerx.Run(ctx, "rm", old.ID); err != nil {
			return fmt.Errorf("web updated; remove retained container %s: %w", backup, err)
		}
	}
	fmt.Fprintf(out, "cxz web installed: %s · %s\n", name, cfg.Origin)
	return nil
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
	return removeWeb(ctx, root, v)
}

func removeWeb(ctx context.Context, root string, v transport.Installation) error {
	c, exists, err := findWeb(ctx, v)
	if err != nil {
		return err
	}
	if exists {
		if _, err = dockerx.Run(ctx, "rm", "-f", c.ID); err != nil {
			return err
		}
	}
	err = os.Remove(filepath.Join(root, "web-installation.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
