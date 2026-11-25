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
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/tailscale/hujson"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// provisionTail is the last thing the devcontainer CLI said before it gave up.
// The CLI reports a failed child command without the output of that child, and
// the output is the only part that says why, so an error that stops at "inspect
// the log" sends every reader on the same round trip. Node stack frames are
// dropped: they name the CLI's own internals, never the cause.
func provisionTail(path string) string {
	const window = 16 << 10
	f, err := os.Open(path)
	if err != nil {
		return "; inspect project provision.log"
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "; inspect project provision.log"
	}
	b := make([]byte, min(window, info.Size()))
	if _, err = f.ReadAt(b, max(0, info.Size()-int64(len(b)))); err != nil && err != io.EOF {
		return "; inspect project provision.log"
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "    at ") {
			continue
		}
		if len(line) > 300 {
			line = line[:300] + "…"
		}
		kept = append(kept, "  "+line)
	}
	if len(kept) == 0 {
		return "; inspect project provision.log"
	}
	return "; the last lines of its output were:\n" + strings.Join(kept[max(0, len(kept)-6):], "\n")
}

// provisionEnv is what the devcontainer CLI runs with.
//
// Docker Compose now delegates builds to buildx bake, and bake refuses to read
// a file outside the Compose project directory unless the build explicitly
// grants it. The CLI wraps every Compose devcontainer in a generated Dockerfile
// under its own temporary directory -- that is how it adds features and the
// base-image stage label -- and points build.dockerfile at that absolute path,
// so the grant is always missing and the build stops with "additional
// privileges requested" before it starts. cxz cannot pass the grant: the CLI
// invokes Compose, not cxz, and compose build takes no --allow.
//
// Turning the check off is scoped to this child process. What it would protect
// is the manager container's own filesystem from a build reading outside its
// context, and the inputs here are the CLI's generated Dockerfile plus a
// configuration the project already had to be trusted to use.
func provisionEnv(project string) []string {
	return []string{"COMPOSE_PROJECT_NAME=" + project, "BUILDX_BAKE_ENTITLEMENTS_FS=0"}
}

func (m *Manager) provision(ctx context.Context, p *Project, kind string, override Override, configFile string) error {
	var e error
	if e = m.checkpoint(ctx, p, "configuration"); e != nil {
		return e
	}
	raw, e := os.ReadFile(configFile)
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
	// The configuration was recorded and decided on by preflight, which read
	// this same file before anything was built.
	base := filepath.Dir(configFile)
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
	if e = m.checkpoint(ctx, p, "resources"); e != nil {
		return e
	}
	assetSource, e := m.prepareAssetRoot(ctx, p)
	if e != nil {
		return e
	}
	secretSource, e := m.prepareSecretRoot(p)
	if e != nil {
		return e
	}
	if e = m.ensureProjectNetwork(ctx, p); e != nil {
		return e
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
	// This directory contains only one project's runtime. The private data
	// subdirectory is created by the devcontainer's remote user, not manager root.
	if p.Volume == "" {
		// The manager owns this filesystem, so it does the two things a helper
		// container used to do: make the directory, and make it writable by the
		// remote user. No container for either.
		dir, _ := m.projectState(p.storageID())
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		if e = os.Chmod(dir, 0o777|os.ModeSticky); e != nil {
			return e
		}
	} else {
		if e = dockerx.EnsureResource(ctx, "volume", p.Volume, m.Owner, p.storageID()); e != nil {
			return e
		}
		if _, e = dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.storageID(), "--entrypoint", "chmod", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state", m.Image, "1777", "/cxz/state"); e != nil {
			return e
		}
	}
	dockerHost, err := m.ensureDocker(ctx, p)
	if err != nil {
		return err
	}
	if e = m.renderProvisionConfig(p, cfg, override, base, dockerHost, secretSource, assetSource); e != nil {
		return e
	}
	configPath := filepath.Join(m.Root, "projects", p.storageID(), "devcontainer.json")
	if e = core.WriteJSON(configPath, cfg); e != nil {
		return e
	}
	// The bootstrap hook can run before the manager knows the remote workspace
	// and selected binaries. It waits for this project-local runtime manifest.
	runtime := Runtime{ProjectID: p.ID, LegacyProjectID: p.StorageID, Workspace: p.RemoteWorkspace, Token: p.Token}
	prev, readErr := m.readRuntime(ctx, p)
	if readErr != nil {
		return readErr
	}
	if len(bytes.TrimSpace(prev)) > 0 {
		var old Runtime
		if e = json.Unmarshal(prev, &old); e != nil {
			return fmt.Errorf("invalid saved runtime: %w", e)
		}
		if !projectid.Same(old.ProjectID, p.ID) || old.Token != p.Token {
			return fmt.Errorf("project volume identity mismatch")
		}
		runtime.Claude, runtime.Codex = old.Claude, old.Codex
	}
	if e = m.writeRuntime(ctx, p, runtime); e != nil {
		return e
	}
	if e = m.checkpoint(ctx, p, "devcontainer-up"); e != nil {
		return e
	}
	result, e := m.devcontainerUp(ctx, p, configFile, configPath, false)
	if e != nil {
		return e
	}
	p.ContainerID = result.ContainerID
	if result.RemoteWorkspace != "" {
		p.RemoteWorkspace = result.RemoteWorkspace
	}
	p.RemoteUser = result.RemoteUser
	if p.RemoteUser == "" {
		p.RemoteUser = "root"
	}
	if _, e = dockerx.Owned(ctx, p.ContainerID, m.Owner, p.storageID()); e != nil {
		return e
	}
	if e = m.installGitConfig(ctx, p); e != nil {
		return e
	}
	if e = m.checkpoint(ctx, p, "agent-tools"); e != nil {
		return e
	}
	platform, e := dockerx.Run(ctx, "exec", p.ContainerID, "uname", "-m")
	if e != nil {
		return e
	}
	libc, _ := dockerx.Run(ctx, "exec", p.ContainerID, "sh", "-c", "ls /lib/ld-musl-*.so.1 2>/dev/null || true")
	if e = m.installGitHub(ctx, p, strings.TrimSpace(string(platform))); e != nil {
		return e
	}
	if e = m.installGitConfig(ctx, p); e != nil {
		return e
	}
	if e = m.checkpoint(ctx, p, "agent-tools"); e != nil {
		return e
	}
	bin, e := distribution.Ensure(ctx, "/cxz/tools", kind, strings.TrimSpace(string(platform)), len(bytes.TrimSpace(libc)) > 0)
	if e != nil {
		return e
	}
	if e = m.verifyAgentBinary(ctx, p, bin); e != nil {
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
	if dockerHost != "" {
		if _, e = dockerx.Run(ctx, "exec", "--user", "root", p.ContainerID, "sh", "-c", `test -x /cxz/tools/docker || { echo 'update cxz install --recreate to install Docker tools' >&2; exit 1; }; mkdir -p /usr/local/bin /usr/local/lib/docker/cli-plugins; if ! command -v docker >/dev/null 2>&1; then ln -s /cxz/tools/docker /usr/local/bin/docker; fi; for plugin in docker-compose docker-buildx; do if [ ! -e /usr/local/lib/docker/cli-plugins/$plugin ]; then ln -s /cxz/tools/docker-cli-plugins/$plugin /usr/local/lib/docker/cli-plugins/$plugin; fi; done`); e != nil {
			return e
		}
	}
	if e = m.checkpoint(ctx, p, "runtime-boot"); e != nil {
		return e
	}
	return m.boot(ctx, p)
}

// renderProvisionConfig decides the project's runtime spec and writes it into
// cfg, the configuration handed to the devcontainer CLI.
func (m *Manager) renderProvisionConfig(p *Project, cfg map[string]any, override Override, base, dockerHost, secretSource, assetSource string) error {
	spec, e := m.runtimeSpec(p, cfg, dockerHost, secretSource, assetSource)
	if e != nil {
		return e
	}
	if p.Volume == "" {
		// A subpath mount requires the directory to be there already, and the
		// manager's own root is that volume.
		dir, _ := m.projectState(p.storageID())
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
	}
	p.RemoteWorkspace = spec.Workspace
	files, e := composeFiles(cfg, base)
	if e != nil {
		return e
	}
	if override.ComposeFile != "" {
		files = append(files, override.ComposeFile)
	}
	compose, e := m.applySpec(cfg, spec, override.Image, files)
	if e != nil {
		return e
	}
	if compose != nil {
		cp := filepath.Join(m.Root, "projects", p.storageID(), "compose.json")
		if e = core.WriteJSON(cp, compose); e != nil {
			return e
		}
		cfg["dockerComposeFile"] = append(files, cp)
	}
	return nil
}

// upResult is what the Dev Container CLI reports about the container it acted
// on. The CLI prints one JSON object per phase, so the last one with an id is
// the answer.
type upResult struct {
	ContainerID     string `json:"containerId"`
	RemoteWorkspace string `json:"remoteWorkspaceFolder"`
	RemoteUser      string `json:"remoteUser"`
}

// devcontainerUp runs the Dev Container CLI against this project.
//
// `existing` is the whole difference between creating and starting. Without it
// the CLI creates what is missing and runs the create-time hooks; with it the
// CLI fails if there is no container, starts the one there is, and runs only
// the start-time hooks. It keeps that distinction itself -- cxz does not have
// to remember which hooks have run. Measured in cxz#274.
func (m *Manager) devcontainerUp(ctx context.Context, p *Project, configFile, configPath string, existing bool) (upResult, error) {
	var out upResult
	cmd := exec.CommandContext(ctx, "devcontainer", m.upArgs(p, configFile, configPath, existing)...)
	cmd.Env = append(os.Environ(), provisionEnv("cxz-"+m.Owner[:12]+"-"+p.storageID())...)
	var stdout bytes.Buffer
	path := filepath.Join(m.Root, "projects", p.storageID(), "provision.log")
	log, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	cmd.Stdout = &stdout
	cmd.Stderr = log
	err = cmd.Run()
	log.Close()
	if err != nil {
		return out, fmt.Errorf("devcontainer up failed: %w%s", err, provisionTail(path))
	}
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		var v upResult
		if json.Unmarshal(line, &v) == nil && v.ContainerID != "" {
			out = v
		}
	}
	if out.ContainerID == "" {
		return out, fmt.Errorf("devcontainer did not return a container id")
	}
	return out, nil
}

// upArgs is how the CLI is asked. `existing` is the only difference between
// the two paths: it makes the CLI refuse to create and run only the start-time
// hooks, which is what keeps `StartContainer` from quietly replacing a
// container or repeating `postCreateCommand`.
func (m *Manager) upArgs(p *Project, configFile, configPath string, existing bool) []string {
	args := []string{"up", "--workspace-folder", p.Workspace, "--config", configFile, "--override-config", configPath, "--id-label", "cxz.owner=" + m.Owner, "--id-label", "cxz.project=" + p.storageID(), "--id-label", "devcontainer.local_folder=" + p.Workspace, "--mount-workspace-git-root=false", "--update-remote-user-uid-default", "off", "--include-merged-configuration"}
	if existing {
		args = append(args, "--expect-existing-container")
	}
	return args
}

// boot starts the project runtime in a container that is already up. The
// runtime reads binary choices every time; its lifetime is not this exec, which
// only starts a detached, independently locked process, so calling it again is
// how a start converges rather than a second runtime.
func (m *Manager) boot(ctx context.Context, p *Project) error {
	_, err := dockerx.Run(ctx, append([]string{"exec", "--user", p.RemoteUser, p.ContainerID}, runtimeStart...)...)
	return err
}
func (m *Manager) writeRuntime(ctx context.Context, p *Project, r Runtime) error {
	if m.kube != nil {
		return m.kubeWriteRuntime(ctx, p, r)
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if p.Volume == "" {
		// The manager owns this filesystem. Same atomic rename, no container.
		dir, _ := m.projectState(p.storageID())
		next := filepath.Join(dir, "runtime.next")
		if err := os.WriteFile(next, b, 0644); err != nil {
			return err
		}
		return os.Rename(next, filepath.Join(dir, "runtime.json"))
	}
	return dockerx.Input(ctx, bytes.NewReader(b), "run", "--rm", "-i", "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.storageID(), "--entrypoint", "sh", "--mount", "type=volume,source="+p.Volume+",target=/cxz/state", m.Image, "-c", "umask 022; cat > /cxz/state/runtime.next && mv /cxz/state/runtime.next /cxz/state/runtime.json")
}

// Validate all readable configuration before a confirmed recreate removes a
// container. Open resolves and snapshots fallback templates before this check.
//
// Every document it reads is recorded in the trust ledger, so the decision
// about what the configuration asks for is made once, by the caller, against
// all of it -- and so the digest that decision is remembered under covers the
// same documents.
func preflight(p *Project, t *trust) error {
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
	if e = t.saw(fmt.Sprintf("configuration %q:", file), cfg); e != nil {
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
		if e = t.saw(fmt.Sprintf("configuration %q:", name), compose); e != nil {
			return e
		}
	}
	return nil
}
