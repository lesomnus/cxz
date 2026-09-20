package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type Engine struct {
	Root, Owner string
	Run         func(context.Context, ...string) ([]byte, error)
}

func (e Engine) Name() string     { return "cxz-" + e.Owner + "-docker" }
func (e Engine) Endpoint() string { return "tcp://" + e.Name() + ":2375" }
func (e Engine) dir() string      { return filepath.Join(e.Root, "docker") }
func (e Engine) Load() (Spec, error) {
	var s Spec
	b, err := os.ReadFile(filepath.Join(e.dir(), "settings.json"))
	if os.IsNotExist(err) {
		return Spec{Mode: "off"}, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, s.Validate()
}
func (e Engine) Save(s Spec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(e.dir(), 0700); err != nil {
		return err
	}
	return core.WriteJSON(filepath.Join(e.dir(), "settings.json"), s)
}
func (e Engine) run(ctx context.Context, args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(ctx, args...)
	}
	return dockerx.Run(ctx, args...)
}
func (e Engine) base(s Spec) map[string]any {
	labels := map[string]string{"cxz.owner": e.Owner, "cxz.role": "docker-engine"}
	name := e.Name()
	return map[string]any{"name": name, "services": map[string]any{"dind": map[string]any{
		"image": s.Image, "container_name": name, "privileged": true, "restart": "unless-stopped",
		"environment": map[string]string{"DOCKER_TLS_CERTDIR": ""},
		"command":     []string{"dockerd", "--host=tcp://0.0.0.0:2375", "--host=unix:///var/run/docker.sock", "--tls=false"},
		"labels":      labels, "networks": []string{"engine"},
		"volumes":     []any{map[string]any{"type": "volume", "source": "data", "target": "/var/lib/docker"}},
		"healthcheck": map[string]any{"test": []string{"CMD", "docker", "-H", "unix:///var/run/docker.sock", "info"}, "interval": "2s", "timeout": "3s", "retries": 30},
	}}, "networks": map[string]any{"engine": map[string]any{"name": name + "-net", "external": true}}, "volumes": map[string]any{"data": map[string]any{"name": name + "-data", "external": true}}}
}

// Render uses Compose's own merge rules, then validates cxz's ownership and
// connection contract. No containers or resources are changed here.
func (e Engine) Render(ctx context.Context, s Spec) ([]byte, string, error) {
	if err := s.Validate(); err != nil {
		return nil, "", err
	}
	if len(e.Owner) != 24 || strings.Trim(e.Owner, "0123456789abcdef") != "" {
		return nil, "", fmt.Errorf("invalid Docker engine owner")
	}
	if err := os.MkdirAll(e.dir(), 0700); err != nil {
		return nil, "", err
	}
	dir, err := os.MkdirTemp(e.dir(), "render-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)
	base := filepath.Join(dir, "base.json")
	if err = core.WriteJSON(base, e.base(s)); err != nil {
		return nil, "", err
	}
	args := []string{"compose", "--project-name", e.Name(), "-f", base}
	if len(s.Override) > 0 {
		p := filepath.Join(dir, "override.json")
		if err = os.WriteFile(p, s.Override, 0600); err != nil {
			return nil, "", err
		}
		args = append(args, "-f", p)
	}
	args = append(args, "config", "--format", "json", "--no-interpolate")
	b, err := e.run(ctx, args...)
	if err != nil {
		return nil, "", err
	}
	var cfg map[string]any
	if err = json.Unmarshal(b, &cfg); err != nil {
		return nil, "", err
	}
	services, _ := cfg["services"].(map[string]any)
	dind, _ := services["dind"].(map[string]any)
	if len(services) != 1 || dind == nil {
		return nil, "", fmt.Errorf("expected exactly one Docker engine service")
	}
	// Resolve and validate host paths before Compose can implicitly create them.
	volumes, _ := dind["volumes"].([]any)
	for _, v := range volumes {
		m, _ := v.(map[string]any)
		source, _ := m["source"].(string)
		if strings.Contains(source, "$") || strings.HasPrefix(source, "~") {
			return nil, "", fmt.Errorf("Docker volume sources must be explicit engine-host paths")
		}
		if m["type"] == "bind" {
			if !filepath.IsAbs(source) || strings.HasPrefix(source, dir+"/") {
				return nil, "", fmt.Errorf("Docker bind sources must be absolute engine-host paths")
			}
			bind, _ := m["bind"].(map[string]any)
			if bind == nil {
				bind = map[string]any{}
			}
			bind["create_host_path"] = false
			m["bind"] = bind
		} else if m["type"] != "volume" || source != "data" {
			return nil, "", fmt.Errorf("additional engine mounts must be absolute bind mounts")
		}
	}
	env, _ := dind["environment"].(map[string]any)
	if env["DOCKER_TLS_CERTDIR"] != "" {
		return nil, "", fmt.Errorf("managed Docker engine requires DOCKER_TLS_CERTDIR to stay empty")
	}
	// Require TCP and Unix endpoints when replacing dockerd flags.
	command, _ := dind["command"].([]any)
	tcp, unix, tls := false, false, false
	for _, arg := range command {
		tcp = tcp || arg == "--host=tcp://0.0.0.0:2375"
		unix = unix || arg == "--host=unix:///var/run/docker.sock"
		tls = tls || arg == "--tls=false"
	}
	if !tcp || !unix || !tls {
		return nil, "", fmt.Errorf("engine command must retain TCP 2375, Unix socket, and --tls=false")
	}
	canonical, err := json.Marshal(cfg)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(canonical)
	revision := hex.EncodeToString(hash[:])
	labels, _ := dind["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
	}
	labels["cxz.docker.spec"] = revision
	dind["labels"] = labels
	canonical, err = json.Marshal(cfg)
	return canonical, revision, err
}
func (e Engine) inspect(ctx context.Context) (*dockerx.Container, error) {
	b, err := e.run(ctx, "ps", "-aq", "--filter", "name=^/"+e.Name()+"$")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, nil
	}
	b, err = e.run(ctx, "inspect", e.Name())
	if err != nil {
		return nil, err
	}
	var all []dockerx.Container
	if json.Unmarshal(b, &all) != nil || len(all) != 1 {
		return nil, fmt.Errorf("invalid engine inspection")
	}
	v := &all[0]
	if v.Config.Labels["cxz.owner"] != e.Owner || v.Config.Labels["cxz.role"] != "docker-engine" || v.Config.Labels["cxz.project"] != "" {
		return nil, fmt.Errorf("refusing unowned Docker engine")
	}
	return v, nil
}
func (e Engine) Ensure(ctx context.Context, s Spec, replace bool) error {
	b, revision, err := e.Render(ctx, s)
	if err != nil {
		return err
	}
	old, err := e.inspect(ctx)
	if err != nil {
		return err
	}
	if old != nil && old.Config.Labels["cxz.docker.spec"] != revision && !replace {
		return fmt.Errorf("Docker engine settings changed; run cxz docker up to apply (restarts engine workloads)")
	}
	for _, r := range []struct{ kind, suffix string }{{"volume", "-data"}, {"network", "-net"}} {
		// The injected runner is used for both verification and creation.
		name := e.Name() + r.suffix
		existing, err := e.run(ctx, r.kind, "ls", "-q", "--filter", "name=^"+name+"$")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(existing)) == "" {
			if _, err = e.run(ctx, r.kind, "create", "--label", "cxz.owner="+e.Owner, "--label", "cxz.role=docker-engine", name); err != nil {
				return err
			}
		}
		raw, err := e.run(ctx, r.kind, "inspect", name)
		if err != nil {
			return err
		}
		var values []struct{ Labels map[string]string }
		if json.Unmarshal(raw, &values) != nil || len(values) != 1 || values[0].Labels["cxz.owner"] != e.Owner || values[0].Labels["cxz.role"] != "docker-engine" {
			return fmt.Errorf("refusing unowned engine %s", r.kind)
		}
	}
	file := filepath.Join(e.dir(), "compose.json")
	if err = os.WriteFile(file, b, 0600); err != nil {
		return err
	}
	_, err = e.run(ctx, "compose", "--project-name", e.Name(), "-f", file, "up", "-d", "--wait", "--wait-timeout", "90", "dind")
	return err
}
func (e Engine) Connect(ctx context.Context, network string) error {
	v, err := e.inspect(ctx)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("Docker engine is absent")
	}
	if _, ok := v.NetworkSettings.Networks[network]; ok {
		return nil
	}
	_, err = e.run(ctx, "network", "connect", network, e.Name())
	return err
}
func (e Engine) Down(ctx context.Context) error {
	v, err := e.inspect(ctx)
	if err != nil || v == nil {
		return err
	}
	_, err = e.run(ctx, "rm", "-f", v.ID)
	return err
}
func (e Engine) Status(ctx context.Context) (string, error) {
	v, err := e.inspect(ctx)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "not running", nil
	}
	if !v.State.Running {
		return "stopped", nil
	}
	return e.Endpoint(), nil
}
