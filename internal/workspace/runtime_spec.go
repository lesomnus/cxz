package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/lesomnus/cxz/internal/githubauth"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/projectspec"
)

// runtimeStart starts the project runtime in a running container. Provisioning
// runs it as the devcontainer's start hook and boot runs it directly; it is
// safe to repeat.
var runtimeStart = []string{"/cxz/tools/cxz", "--state", projectspec.StateDir, "_boot"}

// runtimeSpec is cxz's own requirements for p's container, read from the
// project's devcontainer configuration. It decides; the renderers below only
// write what it decided in the form the devcontainer CLI takes.
func (m *Manager) runtimeSpec(p *Project, cfg map[string]any, dockerHost, secretSource, assetSource string) (projectspec.Spec, error) {
	env := map[string]string{
		"CXZ_PROJECT_ID":     p.ID,
		"CXZ_STATE":          projectspec.StateDir,
		"GH_CONFIG_DIR":      githubauth.ConfigDir,
		"CXZ_SECRET_STORAGE": "host-tmpfs",
	}
	if dockerHost != "" {
		env["DOCKER_HOST"] = dockerHost
		env["DOCKER_CONTEXT"] = ""
		env["DOCKER_TLS_VERIFY"] = ""
		env["DOCKER_CERT_PATH"] = ""
	}
	// A project's state is a subdirectory of the manager's volume, mounted in
	// as /cxz/state. That is what lets the manager read a session's
	// conversation out of the file the session wrote rather than keeping its
	// own copy of every conversation -- and it is the same mechanism the web
	// and relay containers already use for `run` and `pki/server`.
	//
	// A project registered before this still has a volume of its own, and
	// keeps it until a recreate moves it. See cxz#238.
	state := projectspec.Mount{Type: projectspec.Volume, Source: p.Volume, Target: "/cxz/state"}
	if p.Volume == "" {
		if m.StateVolume == "" {
			return projectspec.Spec{}, fmt.Errorf("project %s has no state volume and this manager was installed without one", p.ID)
		}
		_, subpath := m.projectState(p.storageID())
		state = projectspec.Mount{Type: projectspec.Volume, Source: m.StateVolume, Subpath: subpath, Target: "/cxz/state"}
	}
	image, _ := cfg["image"].(string)
	spec := projectspec.Spec{
		Owner:   m.Owner,
		Project: p.storageID(),
		Image:   image,
		Env:     env,
		Mounts: []projectspec.Mount{
			state,
			{Type: projectspec.Volume, Source: m.ToolsVolume, Target: "/cxz/tools", ReadOnly: true},
			{Type: projectspec.Bind, Source: secretSource, Target: "/cxz/secrets"},
			{Type: projectspec.Bind, Source: assetSource, Target: "/cxz/assets", ReadOnly: true},
		},
		Workspace:     remoteWorkspace(cfg, p.Workspace),
		HostWorkspace: p.Workspace,
		Start:         runtimeStart,
		Network:       p.Network,
	}
	return spec, spec.Validate()
}

// remoteWorkspace is where the devcontainer CLI will put the project's files.
func remoteWorkspace(cfg map[string]any, workspace string) string {
	if v, ok := cfg["workspaceFolder"].(string); ok {
		return strings.ReplaceAll(v, "${localWorkspaceFolderBasename}", filepath.Base(workspace))
	}
	return "/workspaces/" + filepath.Base(workspace)
}

// applySpec writes spec into the project's devcontainer configuration. An image
// or Dockerfile devcontainer carries all of it in the configuration; a Compose
// devcontainer gets it as one more Compose file, returned for the caller to
// write and add to dockerComposeFile. composeFiles are the project's Compose
// files plus the shared override's, if any.
func (m *Manager) applySpec(cfg map[string]any, spec projectspec.Spec, shared projectconfig.ImageOverride, composeFiles []any) (map[string]any, error) {
	env := map[string]any{}
	if v, ok := cfg["containerEnv"].(map[string]any); ok {
		env = v
	}
	// The shared override is applied before cxz's own keys below, so a project
	// cannot be handed a devcontainer whose override redefined CXZ_STATE.
	for name, value := range shared.Env {
		env[name] = value
	}
	for _, name := range spec.EnvKeys() {
		env[name] = spec.Env[name]
	}
	cfg["containerEnv"] = env
	hooks := map[string]any{}
	if v, ok := cfg["postStartCommand"]; ok {
		if old, ok := v.(map[string]any); ok {
			hooks = old
		} else {
			hooks["project"] = v
		}
	}
	if _, exists := hooks["cxz-runtime"]; exists {
		return nil, fmt.Errorf("postStartCommand key cxz-runtime is reserved")
	}
	hooks["cxz-runtime"] = strings.Join(spec.Start, " ")
	cfg["postStartCommand"] = hooks
	cfg["workspaceFolder"] = spec.Workspace
	mounts, _ := cfg["mounts"].([]any)
	for _, mount := range shared.Mounts {
		mounts = append(mounts, mount)
	}
	if len(composeFiles) == 0 {
		// Volumes go in mounts, binds of the manager's own directories in
		// runArgs, as they always have.
		runArgs, _ := cfg["runArgs"].([]any)
		runArgs = append(runArgs, "--network", spec.Network)
		for _, mount := range spec.Mounts {
			if mount.Type == projectspec.Volume {
				mounts = append(mounts, mount.String())
			} else {
				runArgs = append(runArgs, "--mount", mount.String())
			}
		}
		cfg["mounts"] = mounts
		cfg["runArgs"] = runArgs
		return nil, nil
	}
	// The CLI's string-mount conversion gives Compose volumes a project prefix
	// and drops readonly, so cxz's mounts are declared in the Compose file.
	cfg["mounts"] = append([]any{}, mounts...)
	return m.composeOverride(cfg, spec, composeFiles)
}

// composeOverride is spec as a Compose file merged last, over the project's.
func (m *Manager) composeOverride(cfg map[string]any, spec projectspec.Spec, files []any) (map[string]any, error) {
	service, _ := cfg["service"].(string)
	if service == "" {
		return nil, fmt.Errorf("compose devcontainer requires service")
	}
	services, devNetworks, err := composeServices(files, service, spec.Labels())
	if err != nil {
		return nil, err
	}
	dev, ok := services[service].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("devcontainer service missing from compose files")
	}
	if len(devNetworks) == 0 {
		devNetworks["default"] = nil
	}
	devNetworks["cxz"] = nil
	dev["networks"] = devNetworks
	environment := map[string]string{}
	for name, value := range spec.Env {
		environment[name] = value
	}
	dev["environment"] = environment
	volumes := []any{}
	external := map[string]any{}
	for _, mount := range spec.Mounts {
		v := map[string]any{"type": string(mount.Type), "source": mount.Source, "target": mount.Target}
		if mount.Subpath != "" {
			v["volume"] = map[string]any{"subpath": mount.Subpath}
		}
		if mount.ReadOnly {
			v["read_only"] = true
		}
		volumes = append(volumes, v)
		if mount.Type == projectspec.Volume {
			external[mount.Source] = map[string]any{"external": true, "name": mount.Source}
		}
	}
	dev["volumes"] = volumes
	return map[string]any{"name": composeProjectName(m.Owner, spec.Project), "services": services, "networks": map[string]any{"cxz": map[string]any{"external": true, "name": spec.Network}}, "volumes": external}, nil
}

// composeServices labels every service of the project's Compose files as
// this project's, and collects the networks the devcontainer service is on.
func composeServices(files []any, service string, labels map[string]string) (map[string]any, map[string]any, error) {
	services := map[string]any{}
	devNetworks := map[string]any{}
	for _, file := range files {
		b, err := os.ReadFile(file.(string))
		if err != nil {
			return nil, nil, err
		}
		var compose map[string]any
		if err = yaml.Unmarshal(b, &compose); err != nil {
			return nil, nil, err
		}
		ss, _ := compose["services"].(map[string]any)
		for name := range ss {
			services[name] = map[string]any{"labels": labels}
			if name != service {
				continue
			}
			spec, ok := ss[name].(map[string]any)
			if !ok {
				continue
			}
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
	return services, devNetworks, nil
}
