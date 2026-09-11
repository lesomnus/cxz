package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/tailscale/hujson"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (m *Manager) provision(ctx context.Context, p *Project, kind string) error {
	var e error
	if p.Config == "" {
		configs := Discover(p.Workspace)
		switch len(configs) {
		case 0:
			p.Config = filepath.Join(p.Workspace, ".devcontainer", "devcontainer.json")
			if e = os.MkdirAll(filepath.Dir(p.Config), 0755); e != nil {
				return e
			}
			if e = core.WriteJSON(p.Config, map[string]any{"name": p.Name, "image": "mcr.microsoft.com/devcontainers/base:bookworm", "remoteUser": "vscode"}); e != nil {
				return e
			}
		case 1:
			p.Config = configs[0]
		default:
			return fmt.Errorf("multiple devcontainer configurations: specify --config (%s)", strings.Join(configs, ", "))
		}
	}
	if !filepath.IsAbs(p.Config) {
		p.Config = filepath.Join(p.Workspace, p.Config)
	}
	raw, e := os.ReadFile(p.Config)
	if e != nil {
		return e
	}
	raw, e = hujson.Standardize(raw)
	if e != nil {
		return e
	}
	var cfg map[string]any
	if e = json.Unmarshal(raw, &cfg); e != nil {
		return e
	}
	if e = CheckTrust(cfg, p.Trusted); e != nil {
		return e
	}
	base := filepath.Dir(p.Config)
	abspath := func(v any) any {
		if s, ok := v.(string); ok && !filepath.IsAbs(s) {
			return filepath.Join(base, s)
		}
		return v
	}
	for _, key := range []string{"dockerFile", "context"} {
		if v, ok := cfg[key]; ok {
			cfg[key] = abspath(v)
		}
	}
	if b, ok := cfg["build"].(map[string]any); ok {
		for _, key := range []string{"dockerfile", "context"} {
			if v, ok := b[key]; ok {
				b[key] = abspath(v)
			}
		}
	}
	for _, name := range []string{p.Network} {
		if e = dockerx.EnsureResource(ctx, "network", name, m.Owner, p.ID); e != nil {
			return e
		}
	}
	if _, e = dockerx.Run(ctx, "network", "connect", p.Network, m.Container); e != nil {
		v, ie := dockerx.Inspect(ctx, m.Container)
		if ie != nil {
			return e
		}
		if _, ok := v.NetworkSettings.Networks[p.Network]; !ok {
			return e
		}
	}
	if e = dockerx.EnsureResource(ctx, "volume", p.Volume, m.Owner, p.ID); e != nil {
		return e
	}
	// This directory contains only one project's runtime. The private data
	// subdirectory is created by the devcontainer's remote user, not manager root.
	if _, e = dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.ID, "--entrypoint", "chmod", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state", m.Image, "1777", "/cxz/state"); e != nil {
		return e
	}
	env := map[string]any{}
	if v, ok := cfg["containerEnv"].(map[string]any); ok {
		env = v
	}
	env["CXZ_PROJECT_ID"] = p.ID
	env["CXZ_STATE"] = "/cxz/state/data"
	cfg["containerEnv"] = env
	mounts, _ := cfg["mounts"].([]any)
	mounts = append(mounts, "type=volume,source="+p.Volume+",target=/cxz/state", "type=volume,source="+m.ToolsVolume+",target=/cxz/tools,readonly")
	cfg["mounts"] = mounts
	hooks := map[string]any{}
	if v, ok := cfg["postStartCommand"]; ok {
		if old, ok := v.(map[string]any); ok {
			hooks = old
		} else {
			hooks["project"] = v
		}
	}
	if _, exists := hooks["cxz-runtime"]; exists {
		return fmt.Errorf("postStartCommand key cxz-runtime is reserved")
	}
	hooks["cxz-runtime"] = "/cxz/tools/cxz --state /cxz/state/data _boot"
	cfg["postStartCommand"] = hooks
	p.RemoteWorkspace = "/workspaces/" + filepath.Base(p.Workspace)
	if v, ok := cfg["workspaceFolder"].(string); ok {
		p.RemoteWorkspace = strings.ReplaceAll(v, "${localWorkspaceFolderBasename}", filepath.Base(p.Workspace))
	}
	cfg["workspaceFolder"] = p.RemoteWorkspace
	files := []any{}
	switch v := cfg["dockerComposeFile"].(type) {
	case string:
		files = append(files, abspath(v))
	case []any:
		for _, f := range v {
			files = append(files, abspath(f))
		}
	}
	if len(files) > 0 {
		// The CLI's string-mount conversion gives Compose volumes a project
		// prefix and drops readonly. Declare our exact external volumes directly.
		cfg["mounts"] = mounts[:len(mounts)-2]
		services := map[string]any{}
		devNetworks := map[string]any{}
		service, _ := cfg["service"].(string)
		if service == "" {
			return fmt.Errorf("compose devcontainer requires service")
		}
		for _, file := range files {
			b, e := os.ReadFile(file.(string))
			if e != nil {
				return e
			}
			var compose map[string]any
			if e = yaml.Unmarshal(b, &compose); e != nil {
				return e
			}
			if e = CheckTrust(compose, p.Trusted); e != nil {
				return e
			}
			ss, _ := compose["services"].(map[string]any)
			for name := range ss {
				services[name] = map[string]any{"labels": map[string]string{"cxz.owner": m.Owner, "cxz.project": p.ID}}
				if name == service {
					if spec, ok := ss[name].(map[string]any); ok {
						switch networks := spec["networks"].(type) {
						case map[string]any:
							for name, options := range networks {
								devNetworks[name] = options
							}
						case []any:
							for _, name := range networks {
								if s, ok := name.(string); ok {
									devNetworks[s] = nil
								}
							}
						}
					}
				}
			}
		}
		dev, ok := services[service].(map[string]any)
		if !ok {
			return fmt.Errorf("devcontainer service missing from compose files")
		}
		if len(devNetworks) == 0 {
			devNetworks["default"] = nil
		}
		devNetworks["cxz"] = nil
		dev["networks"] = devNetworks
		dev["environment"] = map[string]string{"CXZ_PROJECT_ID": p.ID, "CXZ_STATE": "/cxz/state/data"}
		dev["volumes"] = []any{map[string]any{"type": "volume", "source": p.Volume, "target": "/cxz/state"}, map[string]any{"type": "volume", "source": m.ToolsVolume, "target": "/cxz/tools", "read_only": true}}
		override := map[string]any{"name": "cxz-" + m.Owner[:12] + "-" + p.ID, "services": services, "networks": map[string]any{"cxz": map[string]any{"external": true, "name": p.Network}}, "volumes": map[string]any{p.Volume: map[string]any{"external": true, "name": p.Volume}, m.ToolsVolume: map[string]any{"external": true, "name": m.ToolsVolume}}}
		cp := filepath.Join(m.Root, "projects", p.ID, "compose.json")
		if e = core.WriteJSON(cp, override); e != nil {
			return e
		}
		cfg["dockerComposeFile"] = append(files, cp)
	} else {
		runArgs, _ := cfg["runArgs"].([]any)
		runArgs = append(runArgs, "--network", p.Network)
		cfg["runArgs"] = runArgs
	}
	configPath := filepath.Join(m.Root, "projects", p.ID, "devcontainer.json")
	if e = core.WriteJSON(configPath, cfg); e != nil {
		return e
	}
	// The bootstrap hook can run before the manager knows the remote workspace
	// and selected binaries. It waits for this project-local runtime manifest.
	runtime := Runtime{ProjectID: p.ID, Workspace: p.RemoteWorkspace, Token: p.Token}
	prev, readErr := dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.ID, "--entrypoint", "sh", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state,readonly", m.Image, "-c", "if test -f /cxz/state/runtime.json; then cat /cxz/state/runtime.json; fi")
	if readErr != nil {
		return readErr
	}
	if len(bytes.TrimSpace(prev)) > 0 {
		var old Runtime
		if e = json.Unmarshal(prev, &old); e != nil {
			return fmt.Errorf("invalid saved runtime: %w", e)
		}
		if old.ProjectID != p.ID || old.Token != p.Token {
			return fmt.Errorf("project volume identity mismatch")
		}
		runtime.Claude, runtime.Codex = old.Claude, old.Codex
	}
	if e = m.writeRuntime(ctx, p, runtime); e != nil {
		return e
	}
	cmd := exec.CommandContext(ctx, "devcontainer", "up", "--workspace-folder", p.Workspace, "--config", p.Config, "--override-config", configPath, "--id-label", "cxz.owner="+m.Owner, "--id-label", "cxz.project="+p.ID, "--id-label", "devcontainer.local_folder="+p.Workspace, "--mount-workspace-git-root=false", "--update-remote-user-uid-default", "off", "--include-merged-configuration")
	cmd.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME=cxz-"+m.Owner[:12]+"-"+p.ID)
	var stdout bytes.Buffer
	log, e := os.OpenFile(filepath.Join(m.Root, "projects", p.ID, "provision.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	cmd.Stdout = &stdout
	cmd.Stderr = log
	e = cmd.Run()
	log.Close()
	if e != nil {
		return fmt.Errorf("devcontainer up failed: %w; inspect project provision.log", e)
	}
	var result struct {
		ContainerID     string `json:"containerId"`
		RemoteWorkspace string `json:"remoteWorkspaceFolder"`
		RemoteUser      string `json:"remoteUser"`
	}
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		var v struct {
			ContainerID     string `json:"containerId"`
			RemoteWorkspace string `json:"remoteWorkspaceFolder"`
			RemoteUser      string `json:"remoteUser"`
		}
		if json.Unmarshal(line, &v) == nil && v.ContainerID != "" {
			result = v
		}
	}
	if result.ContainerID == "" {
		return fmt.Errorf("devcontainer did not return a container id")
	}
	p.ContainerID = result.ContainerID
	if result.RemoteWorkspace != "" {
		p.RemoteWorkspace = result.RemoteWorkspace
	}
	p.RemoteUser = result.RemoteUser
	if p.RemoteUser == "" {
		p.RemoteUser = "root"
	}
	if _, e = dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID); e != nil {
		return e
	}
	platform, e := dockerx.Run(ctx, "exec", p.ContainerID, "uname", "-m")
	if e != nil {
		return e
	}
	libc, _ := dockerx.Run(ctx, "exec", p.ContainerID, "sh", "-c", "ls /lib/ld-musl-*.so.1 2>/dev/null || true")
	bin, e := distribution.Ensure(ctx, "/cxz/tools", kind, strings.TrimSpace(string(platform)), len(bytes.TrimSpace(libc)) > 0)
	if e != nil {
		return e
	}
	runtime.Workspace = p.RemoteWorkspace
	// Preserve the other vendor's selected binary when switching agent types.
	prev, e = dockerx.Run(ctx, "exec", p.ContainerID, "cat", "/cxz/state/runtime.json")
	if e == nil {
		var old Runtime
		if json.Unmarshal(prev, &old) == nil {
			runtime.Claude = old.Claude
			runtime.Codex = old.Codex
		}
	}
	if kind == "claude" {
		runtime.Claude = bin
	} else {
		runtime.Codex = bin
	}
	if e = m.writeRuntime(ctx, p, runtime); e != nil {
		return e
	}
	// Expose the project-scoped client without replacing an image-provided cxz.
	if _, e = dockerx.Run(ctx, "exec", "--user", "root", p.ContainerID, "sh", "-c", "if ! command -v cxz >/dev/null 2>&1; then mkdir -p /usr/local/bin && ln -s /cxz/tools/cxz /usr/local/bin/cxz; fi"); e != nil {
		return e
	}
	// The runtime reads binary choices on each Create. Its lifetime is not the
	// docker exec connection; _boot only starts a detached, independently locked process.
	_, e = dockerx.Run(ctx, "exec", "--user", p.RemoteUser, p.ContainerID, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_boot")
	return e
}
func (m *Manager) writeRuntime(ctx context.Context, p *Project, r Runtime) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	return dockerx.Input(ctx, bytes.NewReader(b), "run", "--rm", "-i", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.ID, "--entrypoint", "sh", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state", m.Image, "-c", "umask 022; cat > /cxz/state/runtime.next && mv /cxz/state/runtime.next /cxz/state/runtime.json")
}
func CheckTrust(cfg map[string]any, trusted bool) error {
	if trusted {
		return nil
	}
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if (k == "privileged" && v == true) || (k == "initializeCommand") || (k == "network_mode" && v == "host") || (k == "pid" && v == "host") {
					return true
				}
				if walk(v) {
					return true
				}
			}
		case []any:
			for i, v := range x {
				if s, ok := v.(string); ok && (s == "--network" || s == "--net" || s == "--pid") && i+1 < len(x) && x[i+1] == "host" {
					return true
				}
				if walk(v) {
					return true
				}
			}
		case string:
			for _, s := range []string{"docker.sock", "--privileged", "--network=host", "--pid=host", "SYS_ADMIN"} {
				if strings.Contains(x, s) {
					return true
				}
			}
		}
		return false
	}
	if walk(cfg) {
		return fmt.Errorf("devcontainer requests host-side commands or elevated access; inspect the configuration and explicitly pass --trust-config")
	}
	return nil
}

// Validate all readable configuration before a confirmed recreate removes a
// container. Missing default configuration is generated later during provision.
func preflight(p *Project) error {
	file := p.Config
	if file == "" {
		configs := Discover(p.Workspace)
		if len(configs) > 1 {
			return fmt.Errorf("multiple devcontainer configurations: specify --config")
		}
		if len(configs) == 0 {
			return nil
		}
		file = configs[0]
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(p.Workspace, file)
	}
	b, e := os.ReadFile(file)
	if e != nil {
		return e
	}
	b, e = hujson.Standardize(b)
	if e != nil {
		return e
	}
	var cfg map[string]any
	if e = json.Unmarshal(b, &cfg); e != nil {
		return e
	}
	if e = CheckTrust(cfg, p.Trusted); e != nil {
		return e
	}
	var files []any
	switch v := cfg["dockerComposeFile"].(type) {
	case string:
		files = []any{v}
	case []any:
		files = v
	}
	for _, f := range files {
		name, ok := f.(string)
		if !ok {
			return fmt.Errorf("invalid compose filename")
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(filepath.Dir(file), name)
		}
		b, e = os.ReadFile(name)
		if e != nil {
			return e
		}
		var compose map[string]any
		if e = yaml.Unmarshal(b, &compose); e != nil {
			return e
		}
		if e = CheckTrust(compose, p.Trusted); e != nil {
			return e
		}
	}
	return nil
}
