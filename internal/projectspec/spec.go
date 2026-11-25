// Package projectspec is what cxz needs from a project's container, whatever
// runs it. A devcontainer configuration is one way to arrive at a Spec; Docker
// (through the devcontainer CLI) is one way to run it, and a Kubernetes pod is
// meant to be another. See docs/plans/kubernetes-projects.md, cxz#372.
//
// A Spec holds cxz's own requirements. The project's configuration -- its
// image or build, its own environment and mounts, its hooks -- stays the
// project's, and a backend combines the two. Where they meet, cxz wins: a
// project cannot redefine CXZ_STATE or mount over /cxz/state.
package projectspec

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Port is where every project runtime listens for its manager.
const Port = 7348

// StateDir is the runtime's root inside the container: CXZ_STATE.
const StateDir = "/cxz/state/data"

const (
	LabelOwner   = "cxz.owner"
	LabelProject = "cxz.project"
)

type Spec struct {
	// Owner and Project identify the container as this installation's, for
	// this project. Both must match before cxz acts on a container.
	Owner, Project string
	// Image is the prebuilt image the configuration names, or empty when the
	// configuration builds one. Only a backend that can build accepts empty.
	Image string
	// Env is cxz's environment. It is applied after the project's own.
	Env map[string]string
	// Mounts are cxz's own: state, tools, secrets and assets.
	Mounts []Mount
	// Workspace is where the project's files appear inside the container, and
	// HostWorkspace where they come from.
	Workspace, HostWorkspace string
	// Start runs the project runtime once the container is up. It must be
	// safe to repeat: a second start converges on the running runtime.
	Start []string
	// Network is the network the manager reaches the runtime on, at Port.
	Network string
}

type MountType string

const (
	Volume MountType = "volume"
	Bind   MountType = "bind"
)

type Mount struct {
	Type   MountType
	Source string
	// Subpath mounts one directory of a volume rather than the whole volume.
	Subpath  string
	Target   string
	ReadOnly bool
}

// Labels is how a backend marks what it creates for this project.
func (s Spec) Labels() map[string]string {
	return map[string]string{LabelOwner: s.Owner, LabelProject: s.Project}
}

// Validate checks what every backend relies on.
func (s Spec) Validate() error {
	if s.Owner == "" || s.Project == "" {
		return fmt.Errorf("project spec needs an owner and a project")
	}
	if s.Workspace == "" || !strings.HasPrefix(s.Workspace, "/") {
		return fmt.Errorf("project spec needs an absolute workspace path")
	}
	if len(s.Start) == 0 {
		return fmt.Errorf("project spec needs a runtime start command")
	}
	if s.Env["CXZ_STATE"] != StateDir {
		return fmt.Errorf("project spec must set CXZ_STATE=%s", StateDir)
	}
	targets := map[string]bool{}
	for _, m := range s.Mounts {
		if m.Type != Volume && m.Type != Bind {
			return fmt.Errorf("mount %s: unsupported type %q", m.Target, m.Type)
		}
		if m.Source == "" {
			return fmt.Errorf("mount %s has no source", m.Target)
		}
		if m.Subpath != "" && m.Type != Volume {
			return fmt.Errorf("mount %s: only a volume mount has a subpath", m.Target)
		}
		if targets[m.Target] {
			return fmt.Errorf("mount target %s appears twice", m.Target)
		}
		targets[m.Target] = true
	}
	if !targets["/cxz/state"] || !targets["/cxz/tools"] {
		return fmt.Errorf("project spec must mount /cxz/state and /cxz/tools")
	}
	return nil
}

// EnvKeys is Env's names in a stable order.
func (s Spec) EnvKeys() []string { return slices.Sorted(maps.Keys(s.Env)) }

// String is the mount in the form `docker run --mount` takes.
func (m Mount) String() string {
	parts := []string{"type=" + string(m.Type), "source=" + m.Source, "target=" + m.Target}
	if m.Subpath != "" {
		parts = append(parts, "volume-subpath="+m.Subpath)
	}
	if m.ReadOnly {
		parts = append(parts, "readonly")
	}
	return strings.Join(parts, ",")
}
