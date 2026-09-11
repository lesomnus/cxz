package installer

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed image.Dockerfile
var dockerfile []byte

func Build(ctx context.Context, out io.Writer) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	f, e := os.Open(exe)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	h.Write(dockerfile)
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	tag := "cxz-manager:" + hex.EncodeToString(h.Sum(nil))[:16]
	if _, e = dockerx.Run(ctx, "image", "inspect", tag); e == nil {
		return tag, nil
	}
	f.Seek(0, 0)
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	rd, wr := io.Pipe()
	go func() {
		tw := tar.NewWriter(wr)
		e := tw.WriteHeader(&tar.Header{Name: "Dockerfile", Mode: 0644, Size: int64(len(dockerfile))})
		if e == nil {
			_, e = tw.Write(dockerfile)
		}
		if e == nil {
			e = tw.WriteHeader(&tar.Header{Name: "cxz", Mode: 0755, Size: st.Size()})
		}
		if e == nil {
			_, e = io.Copy(tw, f)
		}
		if e == nil {
			e = tw.Close()
		}
		wr.CloseWithError(e)
	}()
	defer rd.Close()
	c := exec.CommandContext(ctx, "docker", "build", "--label", "cxz.role=manager-image", "-t", tag, "-")
	c.Stdin = rd
	c.Stdout = out
	c.Stderr = out
	if e = c.Run(); e != nil {
		return "", e
	}
	return tag, nil
}
func Install(ctx context.Context, root, workspaceRoot, image string, recreate bool, out io.Writer) error {
	if e := core.Prepare(root); e != nil {
		return e
	}
	lock, e := core.Lock(filepath.Join(root, "install.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	v, e := transport.Load(root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if os.IsNotExist(e) {
		owner := core.ID()
		v = transport.Installation{Owner: owner, Container: "cxz-" + owner[:12], StateVolume: "cxz-" + owner + "-state", ToolsVolume: "cxz-" + owner + "-tools"}
	}
	if workspaceRoot == "" {
		workspaceRoot = v.WorkspaceRoot
	}
	if workspaceRoot == "" {
		if host := os.Getenv("DOCKER_HOST"); host != "" && !strings.HasPrefix(host, "unix://") {
			workspaceRoot = "/workspaces"
		} else {
			workspaceRoot, _ = os.UserHomeDir()
		}
	}
	workspaceRoot, e = dockerx.EnginePath(workspaceRoot)
	if e != nil {
		return e
	}
	previousRoot := v.WorkspaceRoot
	v.WorkspaceRoot = workspaceRoot
	// Validate connectivity before replacing any existing manager.
	endpointArgs, e := dockerEndpoint()
	if e != nil {
		return e
	}
	if image == "" {
		image, e = Build(ctx, out)
		if e != nil {
			return e
		}
	}
	previousImage := v.Image
	v.Image = image
	if old, e := dockerx.Inspect(ctx, v.Container); e == nil {
		if old.Config.Labels["cxz.owner"] != v.Owner {
			return fmt.Errorf("daemon name is occupied by an unowned container")
		}
		if !recreate {
			if old.Config.Image != image || previousRoot != workspaceRoot {
				return fmt.Errorf("manager image or workspace root differs; use install --recreate (data retained)")
			}
			if !old.State.Running {
				if _, e = dockerx.Run(ctx, "start", old.ID); e != nil {
					return e
				}
			}
			return waitReady(ctx, v, out)
		}
		if previousImage != image && previousImage != "" {
			v.PreviousImage = previousImage
		}
		if e = core.WriteJSON(filepath.Join(root, "installation.json"), v); e != nil {
			return e
		}
		if _, e = dockerx.Run(ctx, "rm", "-f", old.ID); e != nil {
			return e
		}
	}
	// Persist identity before creating resources, including on a first attempt.
	if e = core.WriteJSON(filepath.Join(root, "installation.json"), v); e != nil {
		return e
	}
	for _, vol := range []string{v.StateVolume, v.ToolsVolume} {
		if e = dockerx.EnsureResource(ctx, "volume", vol, v.Owner, ""); e != nil {
			return e
		}
	}
	// Existing credentials stay on the daemon/project volumes. Only the helper
	// binary is refreshed, via atomic rename so running project processes survive.
	if _, e = dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+v.Owner, "--entrypoint", "sh", "--mount", "type=volume,source="+v.ToolsVolume+",target=/cxz/tools", image, "-c", "cp /usr/local/bin/cxz /cxz/tools/cxz.next && chmod 755 /cxz/tools/cxz.next && mv /cxz/tools/cxz.next /cxz/tools/cxz"); e != nil {
		return e
	}
	args := []string{"run", "-d", "--name", v.Container, "--restart", "unless-stopped", "--label", "cxz.role=daemon", "--label", "cxz.owner=" + v.Owner, "--mount", "type=volume,source=" + v.StateVolume + ",target=/var/lib/cxz", "--mount", "type=volume,source=" + v.ToolsVolume + ",target=/cxz/tools", "--mount", "type=bind,source=" + workspaceRoot + ",target=" + workspaceRoot, "-e", "CXZ_OWNER=" + v.Owner, "-e", "CXZ_WORKSPACE_ROOT=" + workspaceRoot, "-e", "CXZ_TOOLS_VOLUME=" + v.ToolsVolume, "-e", "CXZ_MANAGER_IMAGE=" + image, "-e", "CXZ_MANAGER_CONTAINER=" + v.Container}
	args = append(args, "-e", "CXZ_HOST_UID="+strconv.Itoa(os.Getuid()), "-e", "CXZ_HOST_GID="+strconv.Itoa(os.Getgid()))
	args = append(args, endpointArgs...)
	args = append(args, image, "--state", "/var/lib/cxz", "serve")
	if _, e = dockerx.Run(ctx, args...); e != nil {
		return e
	}
	return waitReady(ctx, v, out)
}

func dockerEndpoint() ([]string, error) {
	var args []string
	host := os.Getenv("DOCKER_HOST")
	if host == "" || strings.HasPrefix(host, "unix://") {
		path := strings.TrimPrefix(host, "unix://")
		if path == "" {
			path = "/var/run/docker.sock"
		}
		args = append(args, "--mount", "type=bind,source="+path+",target=/var/run/docker.sock", "-e", "DOCKER_HOST=unix:///var/run/docker.sock")
	} else {
		u, e := url.Parse(host)
		if e != nil || u.Scheme != "tcp" {
			return nil, fmt.Errorf("unsupported Docker endpoint")
		}
		if os.Getenv("DOCKER_TLS_VERIFY") != "" {
			return nil, fmt.Errorf("TLS Docker endpoints need an explicit configured manager image/mount setup")
		}
		ips, e := net.LookupHost(u.Hostname())
		if e != nil {
			return nil, e
		}
		u.Host = net.JoinHostPort(ips[0], u.Port())
		args = append(args, "-e", "DOCKER_HOST="+u.String())
	}
	return args, nil
}

func waitReady(ctx context.Context, v transport.Installation, out io.Writer) error {
	for i := 0; i < 150; i++ {
		if e := dockerx.Input(ctx, bytes.NewReader(nil), "exec", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_ready"); e == nil {
			fmt.Fprintln(out, "cxz installed:", v.Container, "workspace root:", v.WorkspaceRoot)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("daemon did not become ready; inspect docker logs %s", v.Container)
}
func Uninstall(ctx context.Context, root string) error {
	v, e := transport.Load(root)
	if e != nil {
		return e
	}
	c, e := dockerx.Inspect(ctx, v.Container)
	if e != nil {
		return e
	}
	if c.Config.Labels["cxz.owner"] != v.Owner {
		return fmt.Errorf("refusing unowned daemon")
	}
	if _, e = dockerx.Run(ctx, "rm", "-f", c.ID); e != nil {
		return e
	}
	return nil /* locator and named volumes deliberately retained for install */
}
