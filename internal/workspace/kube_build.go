package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KubeBuild is where an installation builds project images. cxz runs neither:
// both are the installation's, named here. Builds are offered when both are.
type KubeBuild struct {
	// BuildKit is the builder's address, as buildctl takes it: tcp://host:port.
	BuildKit string `json:"buildkit"`
	// Registry is where images are pushed: host[:port]/path.
	Registry string `json:"registry"`
	// PullRegistry is how the nodes name the same registry, when that differs
	// from how the builder reaches it. Empty is Registry.
	PullRegistry string `json:"pull_registry,omitempty"`
	// AuthSecret is a kubernetes.io/dockerconfigjson Secret: the builder
	// pushes with it and project pods pull with it.
	AuthSecret string `json:"auth_secret,omitempty"`
	// Insecure pushes over plain HTTP.
	Insecure bool `json:"insecure,omitempty"`
}

func (b KubeBuild) enabled() bool { return b.BuildKit != "" && b.Registry != "" }

func (p *Project) kubeImageRepository(registry string) string {
	name := p.Name
	if p.Workspace == p.ID {
		name = p.ID
	}
	return strings.TrimRight(registry, "/") + "/" + name
}

// ProjectBuild is what a project's image is built from, in its workspace.
type ProjectBuild struct {
	Dockerfile string `json:"dockerfile,omitempty"`
	Context    string `json:"context,omitempty"`
}

// buildctlPath is the builder's client inside a project pod, copied there
// from the manager's image with cxz.
const buildctlPath = "/cxz/tools/buildctl"

// workspacePath is a path inside the workspace, refused if it leaves it.
func workspacePath(p string) (string, error) {
	clean := path.Clean(p)
	if p == "" || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%q is not a path inside the workspace", p)
	}
	return clean, nil
}

func (m *Manager) BuildProject(ctx context.Context, r *api.BuildProjectInput) (*api.Receipt, error) {
	if m.kube == nil {
		return nil, status.Error(codes.FailedPrecondition, "image builds are for projects on Kubernetes; a Docker project builds from its devcontainer configuration")
	}
	settings := m.kubeSettings.Build
	if !settings.enabled() {
		return nil, status.Error(codes.FailedPrecondition, "no image builder is configured: set build.buildkit and build.registry in the Kubernetes settings")
	}
	p, err := m.resolve(ctx, r.Workspace)
	if err != nil {
		return nil, err
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	defer lock.Unlock()
	if p, err = m.resolve(ctx, p.ID); err != nil {
		return nil, err
	}
	if p.Kube == nil {
		p.Kube = &KubeProject{}
	}
	build := ProjectBuild{Dockerfile: "Dockerfile", Context: "."}
	if p.Kube.Build != nil {
		build = *p.Kube.Build
	}
	if r.Dockerfile != "" {
		build.Dockerfile = r.Dockerfile
	}
	if r.Context != "" {
		build.Context = r.Context
	}
	if build.Dockerfile, err = workspacePath(build.Dockerfile); err != nil {
		return nil, status.Error(codes.InvalidArgument, "dockerfile: "+err.Error())
	}
	if build.Context, err = workspacePath(build.Context); err != nil {
		return nil, status.Error(codes.InvalidArgument, "context: "+err.Error())
	}
	if _, err = m.kubeAddress(ctx, p); err != nil {
		return nil, status.Error(codes.FailedPrecondition, "start the project first: the build runs in its pod, where the workspace is")
	}
	if _, err = m.containerRun(ctx, p.ContainerID, "test", "-x", buildctlPath); err != nil {
		return nil, status.Error(codes.FailedPrecondition, "this project's pod has no buildctl; restart it with a manager image that carries one")
	}
	repository := p.kubeImageRepository(settings.Registry)
	tag := time.Now().UTC().Format("20060102-150405")
	output := "type=image,name=" + repository + ":" + tag + ",push=true"
	if settings.Insecure {
		output += ",registry.insecure=true"
	}
	metadata := "/cxz/state/build-metadata.json"
	args := []string{"--user", p.RemoteUser, "--workdir", p.RemoteWorkspace}
	if settings.AuthSecret != "" {
		args = append(args, "--env", "DOCKER_CONFIG=/cxz/registry")
	}
	args = append(args, p.ContainerID, buildctlPath, "--addr", settings.BuildKit, "build",
		"--frontend", "dockerfile.v0",
		"--local", "context="+build.Context,
		"--local", "dockerfile="+path.Dir(build.Dockerfile),
		"--opt", "filename="+path.Base(build.Dockerfile),
		"--output", output,
		"--metadata-file", metadata)
	logPath := filepath.Join(m.Root, "projects", p.storageID(), "build.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	cmd := m.containerCommand(ctx, args...)
	cmd.Stdout, cmd.Stderr = log, log
	err = cmd.Run()
	log.Close()
	if err != nil {
		return nil, fmt.Errorf("image build failed: %w%s", err, provisionTail(logPath))
	}
	raw, err := m.containerRun(ctx, p.ContainerID, "cat", metadata)
	if err != nil {
		return nil, fmt.Errorf("image build: read its result: %w", err)
	}
	var result struct {
		Digest string `json:"containerimage.digest"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || !strings.HasPrefix(result.Digest, "sha256:") {
		return nil, fmt.Errorf("image build did not report a digest")
	}
	pull := settings.PullRegistry
	if pull == "" {
		pull = settings.Registry
	}
	// By digest: the image the build made, whatever the tag points to later.
	p.Kube.Build = &build
	p.Kube.Built = p.kubeImageRepository(pull) + "@" + result.Digest
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	// The workspace and sessions are on the volume; only the pod is replaced.
	m.quiesce(ctx, p)
	if err = m.kubeDeletePod(ctx, p); err != nil {
		return nil, err
	}
	if err = m.kubeUp(ctx, p); err != nil {
		return nil, err
	}
	p.Error = ""
	p.Job.State, p.Job.Step = "complete", "ready"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "built " + p.Kube.Built}, core.SyncDir(filepath.Dir(logPath))
}
