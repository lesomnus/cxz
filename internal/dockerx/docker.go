package dockerx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Run(ctx context.Context, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "docker", args...)
	var errout bytes.Buffer
	c.Stderr = &errout
	b, e := c.Output()
	if e != nil {
		return nil, fmt.Errorf("docker %s: %w: %.4000s", args[0], e, errout.String())
	}
	return b, nil
}
func Input(ctx context.Context, in io.Reader, args ...string) error {
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdin = in
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = &b
	if e := c.Run(); e != nil {
		return fmt.Errorf("docker %s: %w: %.4000s", args[0], e, b.String())
	}
	return nil
}

type Container struct {
	ID     string `json:"Id"`
	Name   string
	Config struct {
		Labels map[string]string
		User   string
	}
	State           struct{ Running bool }
	NetworkSettings struct {
		Networks map[string]struct{ IPAddress string }
	}
	Mounts     []struct{ Source, Destination, Type, Name string }
	HostConfig struct {
		Privileged  bool
		NetworkMode string
	}
}

func Inspect(ctx context.Context, id string) (Container, error) {
	var v []Container
	b, e := Run(ctx, "inspect", id)
	if e != nil {
		return Container{}, e
	}
	if e = json.Unmarshal(b, &v); e != nil || len(v) != 1 {
		return Container{}, fmt.Errorf("invalid Docker inspect: %v", e)
	}
	return v[0], nil
}
func List(ctx context.Context, filters ...string) ([]Container, error) {
	args := []string{"ps", "-aq"}
	for _, f := range filters {
		args = append(args, "--filter", f)
	}
	b, e := Run(ctx, args...)
	if e != nil {
		return nil, e
	}
	var out []Container
	for _, id := range strings.Fields(string(b)) {
		v, e := Inspect(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func Owned(ctx context.Context, id, owner, project string) (Container, error) {
	v, e := Inspect(ctx, id)
	if e != nil {
		return v, e
	}
	if v.Config.Labels["cxz.owner"] != owner || v.Config.Labels["cxz.project"] != project {
		return v, fmt.Errorf("refusing operation on container not owned by this cxz project")
	}
	return v, nil
}

// EnsureResource never adopts a pre-existing shared-engine volume or network.
func EnsureResource(ctx context.Context, kind, name, owner, project string) error {
	if kind != "volume" && kind != "network" {
		return fmt.Errorf("unsupported resource kind")
	}
	b, e := Run(ctx, kind, "inspect", name)
	if e == nil {
		var resources []struct{ Labels map[string]string }
		if json.Unmarshal(b, &resources) != nil || len(resources) != 1 || resources[0].Labels["cxz.owner"] != owner || resources[0].Labels["cxz.project"] != project {
			return fmt.Errorf("refusing unowned %s %s", kind, name)
		}
		return nil
	}
	args := []string{kind, "create", "--label", "cxz.owner=" + owner}
	if project != "" {
		args = append(args, "--label", "cxz.project="+project)
	}
	if _, e = Run(ctx, append(args, name)...); e != nil {
		return e
	}
	// Verify again: volume create is idempotent even if an unowned name raced us.
	b, e = Run(ctx, kind, "inspect", name)
	if e != nil {
		return e
	}
	var resources []struct{ Labels map[string]string }
	if json.Unmarshal(b, &resources) != nil || len(resources) != 1 || resources[0].Labels["cxz.owner"] != owner || resources[0].Labels["cxz.project"] != project {
		return fmt.Errorf("resource ownership mismatch")
	}
	return nil
}

// EnginePath translates this development environment's documented shared bind.
// Unknown remote paths fail rather than silently mounting an empty directory.
func EnginePath(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	host := os.Getenv("DOCKER_HOST")
	if host == "" || strings.HasPrefix(host, "unix://") || p == "/workspaces" || strings.HasPrefix(p, "/workspaces/") {
		return p, nil
	}
	if p == "/workspace" || strings.HasPrefix(p, "/workspace/") {
		st, e := os.Stat("/workspace")
		if e != nil {
			return "", e
		}
		candidates, _ := filepath.Glob("/workspaces/*/*/*")
		for _, v := range candidates {
			other, e := os.Stat(v)
			if e == nil && os.SameFile(st, other) {
				rel, _ := filepath.Rel("/workspace", p)
				return filepath.Join(v, rel), nil
			}
		}
	}
	return "", fmt.Errorf("remote Docker engine cannot resolve %s; use a shared /workspaces path (or an explicit engine-visible path)", p)
}
