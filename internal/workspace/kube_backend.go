package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/kube"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/projectspec"
	"google.golang.org/grpc"
)

// KubeSettings is how an installation on Kubernetes runs its projects. It is
// the backend's own configuration: a pod is not a devcontainer, and nothing
// here is translated from one. Each project may override the image, user,
// storage and resources (KubeProject).
type KubeSettings struct {
	// Image is the project container's image when a project names none.
	Image string `json:"image"`
	// User runs the runtime and every session; empty is the image's user.
	User string `json:"user"`
	// Storage and StorageClass size the project's volume. It holds the
	// runtime's state, the workspace and the project's own tools.
	Storage      string `json:"storage"`
	StorageClass string `json:"storage_class"`
	// Requests and Limits are container resources, e.g. {"cpu": "1", "memory": "2Gi"}.
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
	// ToolsImage carries the cxz that is copied into each project; it is the
	// manager's own image.
	ToolsImage      string `json:"tools_image"`
	ImagePullPolicy string `json:"image_pull_policy"`
	// Build names the installation's image builder and registry.
	Build KubeBuild `json:"build,omitempty"`
	// ReadyTimeout bounds waiting for a pod: image pull, volume and runtime.
	ReadyTimeout string `json:"ready_timeout"`
	// RuntimeRollout is what happens to a project pod whose runtime is an
	// older manager's: "idle" replaces it once its sessions are idle, "manual"
	// only reports it. See kube_rollout.go.
	RuntimeRollout string `json:"runtime_rollout,omitempty"`
	// PodTemplates name PodTemplates in the namespace merged into every
	// project's pod, in order: a credential, a node selector. See
	// kube_podtemplate.go.
	PodTemplates []string `json:"pod_templates,omitempty"`
}

// KubeProject is one project's choices, over the installation's.
type KubeProject struct {
	// Built is the image this project's last build pushed, by digest. It
	// takes precedence over Image.
	Built    string            `json:"built,omitempty"`
	Build    *ProjectBuild     `json:"build,omitempty"`
	Image    string            `json:"image,omitempty"`
	User     string            `json:"user,omitempty"`
	Storage  string            `json:"storage,omitempty"`
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
	// PodTemplates follow the installation's; PodTemplate, a PodTemplate's
	// template inline, is merged last.
	PodTemplates []string       `json:"pod_templates,omitempty"`
	PodTemplate  map[string]any `json:"pod_template,omitempty"`
}

func loadKubeSettings() (KubeSettings, error) {
	s := KubeSettings{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", User: "vscode", Storage: "20Gi", ToolsImage: os.Getenv("CXZ_MANAGER_IMAGE"), ImagePullPolicy: "IfNotPresent", ReadyTimeout: "10m"}
	if raw := os.Getenv("CXZ_KUBERNETES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return s, fmt.Errorf("CXZ_KUBERNETES: %w", err)
		}
	}
	if _, err := time.ParseDuration(s.ReadyTimeout); err != nil {
		return s, fmt.Errorf("CXZ_KUBERNETES ready_timeout: %w", err)
	}
	switch s.RuntimeRollout {
	case "":
		s.RuntimeRollout = rolloutIdle
	case rolloutIdle, rolloutManual:
	default:
		return s, fmt.Errorf("CXZ_KUBERNETES runtime_rollout: %q is neither %q nor %q", s.RuntimeRollout, rolloutIdle, rolloutManual)
	}
	return s, nil
}

// useKubernetes connects the manager to the cluster it runs in, when its
// installation says so. Docker is then never asked for projects.
func (m *Manager) useKubernetes() error {
	if os.Getenv("CXZ_BACKEND") != "kubernetes" {
		return nil
	}
	settings, err := loadKubeSettings()
	if err != nil {
		return err
	}
	cfg, err := kube.Load()
	if err != nil {
		return err
	}
	client, err := kube.New(cfg)
	if err != nil {
		return err
	}
	if settings.ToolsImage == "" {
		if settings.ToolsImage, err = ownImage(context.Background(), client); err != nil {
			return fmt.Errorf("kubernetes backend needs the manager's own image for project pods (tools_image): %w", err)
		}
	}
	m.kube, m.kubeSettings = client, settings
	return nil
}

// ownImage is the image this manager runs from, read from its own pod, so a
// project copies the same cxz whatever tag an overlay set. By digest when the
// node reports one.
func ownImage(ctx context.Context, c *kube.Client) (string, error) {
	name, err := os.Hostname()
	if err != nil {
		return "", err
	}
	var pod struct {
		Spec struct {
			Containers []struct{ Name, Image string } `json:"containers"`
		} `json:"spec"`
		Status struct {
			ContainerStatuses []struct {
				Name    string `json:"name"`
				ImageID string `json:"imageID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	}
	if err = c.Get(ctx, c.Path("pods", name), &pod); err != nil {
		return "", err
	}
	for _, container := range pod.Spec.Containers {
		if container.Name != "manager" {
			continue
		}
		for _, s := range pod.Status.ContainerStatuses {
			// A registry's digest pins it; a locally imported image has none.
			if s.Name == "manager" && strings.Contains(s.ImageID, "@sha256:") {
				return strings.TrimPrefix(s.ImageID, "docker-pullable://"), nil
			}
		}
		return container.Image, nil
	}
	return "", fmt.Errorf("pod %s has no manager container", name)
}

// Kubernetes reports whether projects run as pods.
func (m *Manager) Kubernetes() bool { return m.kube != nil }

var kubeName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// kubeResource is the one name a project's pod, volume and secret share.
func (m *Manager) kubeResource(p *Project) string {
	return "cxz-" + m.Owner[:12] + "-" + p.storageID()
}

func (m *Manager) kubeProject(p *Project) (image, user, storage string, requests, limits map[string]string) {
	s := m.kubeSettings
	image, user, storage, requests, limits = s.Image, s.User, s.Storage, s.Requests, s.Limits
	if k := p.Kube; k != nil {
		if k.Image != "" {
			image = k.Image
		}
		if k.Built != "" {
			image = k.Built
		}
		if k.User != "" {
			user = k.User
		}
		if k.Storage != "" {
			storage = k.Storage
		}
		if k.Requests != nil {
			requests = k.Requests
		}
		if k.Limits != nil {
			limits = k.Limits
		}
	}
	return
}

// kubeRegister records a project by name: there is no host folder to point
// at, and the workspace is the project's own volume. config, when given, is
// the project's KubeProject as YAML, JSON or an image reference.
func (m *Manager) kubeRegister(ctx context.Context, name, config string) (*Project, error) {
	if !kubeName.MatchString(name) {
		return nil, fmt.Errorf("a Kubernetes project name is 1-40 lowercase letters, digits or hyphens")
	}
	k, err := parseKubeProject(config)
	if err != nil {
		return nil, fmt.Errorf("project settings: %w", err)
	}
	if err := validPodTemplate(k.PodTemplate); err != nil {
		return nil, err
	}
	if len(m.Owner) < 12 {
		return nil, fmt.Errorf("invalid manager owner")
	}
	lock := m.projectLock(name)
	lock.Lock()
	defer lock.Unlock()
	if p, err := m.resolve(ctx, name); err == nil {
		return p, nil
	}
	p := &Project{ID: projectid.New(), Workspace: "/workspaces/" + name, Name: name, RemoteWorkspace: "/workspaces/" + name, Token: core.ID() + core.ID(), Kube: k}
	return p, m.save(ctx, p)
}

func parseKubeProject(config string) (*KubeProject, error) {
	k := &KubeProject{}
	raw := []byte(strings.TrimSpace(config))
	if len(raw) == 0 {
		return k, nil
	}
	if raw[0] == '{' && json.Valid(raw) {
		return k, json.Unmarshal(raw, k)
	}
	file, err := parser.ParseBytes(raw, 0)
	if err != nil {
		return nil, err
	}
	if len(file.Docs) > 1 {
		return nil, fmt.Errorf("expected a single YAML document")
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return k, nil
	}
	var value any
	if err := yaml.NodeToValue(file.Docs[0].Body, &value); err != nil {
		return nil, err
	}
	switch value := value.(type) {
	case string:
		if strings.IndexFunc(value, unicode.IsSpace) < 0 {
			k.Image = value
			return k, nil
		}
	case map[string]any:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		return k, decoder.Decode(k)
	}
	return nil, fmt.Errorf("expected a settings mapping or an image reference")
}

type kubePod struct {
	Metadata struct {
		Name              string            `json:"name"`
		Labels            map[string]string `json:"labels"`
		Annotations       map[string]string `json:"annotations"`
		DeletionTimestamp string            `json:"deletionTimestamp"`
	} `json:"metadata"`
	Spec struct {
		InitContainers []struct {
			Name  string `json:"name"`
			Image string `json:"image"`
		} `json:"initContainers"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		PodIP             string `json:"podIP"`
		Reason            string `json:"reason"`
		Message           string `json:"message"`
		ContainerStatuses []struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
			State struct {
				Waiting *struct {
					Reason  string `json:"reason"`
					Message string `json:"message"`
				} `json:"waiting"`
				Terminated *struct {
					Reason   string `json:"reason"`
					ExitCode int    `json:"exitCode"`
				} `json:"terminated"`
			} `json:"state"`
		} `json:"containerStatuses"`
		InitContainerStatuses []struct {
			Name  string `json:"name"`
			State struct {
				Waiting *struct {
					Reason  string `json:"reason"`
					Message string `json:"message"`
				} `json:"waiting"`
				Terminated *struct {
					Reason   string `json:"reason"`
					ExitCode int    `json:"exitCode"`
				} `json:"terminated"`
			} `json:"state"`
		} `json:"initContainerStatuses"`
	} `json:"status"`
}

func (p kubePod) ready() bool {
	if p.Status.Phase != "Running" || p.Metadata.DeletionTimestamp != "" {
		return false
	}
	for _, c := range p.Status.ContainerStatuses {
		if c.Name == "runtime" {
			return c.Ready
		}
	}
	return false
}

// problem is why a pod cannot become ready, when it says so: an image that
// will not pull, a crashing container. Empty while it is only slow.
func (p kubePod) problem() string {
	for _, c := range p.Status.InitContainerStatuses {
		if w := c.State.Waiting; w != nil && (strings.Contains(w.Reason, "Err") || w.Reason == "CrashLoopBackOff") {
			return c.Name + ": " + w.Reason + ": " + w.Message
		}
		if t := c.State.Terminated; t != nil && t.ExitCode != 0 {
			return c.Name + ": " + t.Reason + fmt.Sprintf(" (exit %d)", t.ExitCode)
		}
	}
	for _, c := range p.Status.ContainerStatuses {
		if w := c.State.Waiting; w != nil && (strings.Contains(w.Reason, "Err") || w.Reason == "CrashLoopBackOff") {
			return c.Name + ": " + w.Reason + ": " + w.Message
		}
	}
	if p.Status.Phase == "Failed" {
		return "pod failed: " + p.Status.Reason + " " + p.Status.Message
	}
	return ""
}

func (m *Manager) kubePod(ctx context.Context, p *Project) (*kubePod, error) {
	var pod kubePod
	err := m.kube.Get(ctx, m.kube.Path("pods", m.kubeResource(p)), &pod)
	if kube.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The name is cxz's, but the labels are what the pod says it is: both
	// must be this installation's and this project's, as for a container.
	if pod.Metadata.Labels[projectspec.LabelOwner] != m.Owner || pod.Metadata.Labels[projectspec.LabelProject] != p.storageID() {
		return nil, fmt.Errorf("refusing pod %s: not owned by this cxz project", m.kubeResource(p))
	}
	return &pod, nil
}

func (m *Manager) kubeLabels(p *Project) map[string]string {
	return map[string]string{projectspec.LabelOwner: m.Owner, projectspec.LabelProject: p.storageID(), "app.kubernetes.io/managed-by": "cxz", "app.kubernetes.io/component": "project"}
}

// kubeRuntimeSecret holds the runtime manifest, which carries the project's
// token. The pod copies it onto its volume before the runtime starts.
func (m *Manager) kubeWriteRuntime(ctx context.Context, p *Project, r Runtime) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	secret := map[string]any{
		"apiVersion": "v1", "kind": "Secret",
		"metadata":   map[string]any{"name": m.kubeResource(p), "labels": m.kubeLabels(p)},
		"type":       "Opaque",
		"stringData": map[string]string{"runtime.json": string(b)},
	}
	if err = m.kube.Apply(ctx, m.kube.Path("secrets", m.kubeResource(p)), secret, nil); err != nil {
		return err
	}
	// A running runtime reads its binary choices from the file every time.
	if pod, err := m.kubePod(ctx, p); err == nil && pod != nil && pod.ready() {
		cmd := m.containerCommand(ctx, "-i", "--user", p.RemoteUser, m.kubeResource(p), "sh", "-c", "umask 022; cat > /cxz/state/runtime.next && mv /cxz/state/runtime.next /cxz/state/runtime.json")
		cmd.Stdin = bytes.NewReader(b)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("update runtime manifest in pod: %w: %s", err, out)
		}
	}
	return nil
}

func (m *Manager) kubeReadRuntime(ctx context.Context, p *Project) ([]byte, error) {
	var secret struct {
		Data map[string][]byte `json:"data"`
	}
	err := m.kube.Get(ctx, m.kube.Path("secrets", m.kubeResource(p)), &secret)
	if kube.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return secret.Data["runtime.json"], nil
}

// kubeUp makes the project's pod run: volume, manifest and pod, each declared
// rather than created, so a second Up converges on the first. It returns
// once the runtime answers.
func (m *Manager) kubeUp(ctx context.Context, p *Project) error {
	image, user, storage, requests, limits := m.kubeProject(p)
	name := m.kubeResource(p)
	labels := m.kubeLabels(p)
	pvc := map[string]any{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]any{"name": name, "labels": labels},
		"spec": map[string]any{
			"accessModes": []string{"ReadWriteOnce"},
			"resources":   map[string]any{"requests": map[string]string{"storage": storage}},
		},
	}
	if m.kubeSettings.StorageClass != "" {
		pvc["spec"].(map[string]any)["storageClassName"] = m.kubeSettings.StorageClass
	}
	// A claim's size and class are fixed once bound; declare it only once.
	var existing map[string]any
	if err := m.kube.Get(ctx, m.kube.Path("persistentvolumeclaims", name), &existing); kube.IsNotFound(err) {
		if err = m.kube.Apply(ctx, m.kube.Path("persistentvolumeclaims", name), pvc, nil); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	runtime := Runtime{ProjectID: p.ID, LegacyProjectID: p.StorageID, Workspace: p.RemoteWorkspace, Token: p.Token}
	if prev, err := m.kubeReadRuntime(ctx, p); err != nil {
		return err
	} else if len(prev) > 0 {
		var old Runtime
		if err = json.Unmarshal(prev, &old); err != nil {
			return fmt.Errorf("invalid saved runtime: %w", err)
		}
		runtime.Claude, runtime.Codex = old.Claude, old.Codex
	}
	if err := m.kubeWriteRuntime(ctx, p, runtime); err != nil {
		return err
	}
	pod, err := m.kubePod(ctx, p)
	if err != nil {
		return err
	}
	if pod != nil && (pod.Status.Phase == "Failed" || pod.Status.Phase == "Succeeded") {
		if err = m.kubeDeletePod(ctx, p); err != nil {
			return err
		}
		pod = nil
	}
	if pod == nil {
		spec, err := m.kubeSpec(p)
		if err != nil {
			return err
		}
		templates, err := m.kubePodTemplates(ctx, p)
		if err != nil {
			return err
		}
		obj, err := applyPodTemplates(m.kubePodObject(p, spec, image, user, requests, limits), templates)
		if err != nil {
			return err
		}
		if err = m.kube.Apply(ctx, m.kube.Path("pods", name), obj, nil); err != nil {
			return err
		}
	}
	if err = m.kubeWaitReady(ctx, p); err != nil {
		return err
	}
	p.ContainerID = name
	p.RemoteUser = user
	if p.RemoteUser == "" {
		p.RemoteUser = "root"
	}
	return nil
}

// kubeSpec is cxz's requirements for the project's container. The mounts it
// names are satisfied by the pod's volume and emptyDirs, not Docker volumes.
func (m *Manager) kubeSpec(p *Project) (projectspec.Spec, error) {
	spec := projectspec.Spec{
		Owner: m.Owner, Project: p.storageID(),
		Env: map[string]string{
			"CXZ_PROJECT_ID":     p.ID,
			"CXZ_STATE":          projectspec.StateDir,
			"CXZ_SECRET_STORAGE": "pod-memory",
		},
		Mounts: []projectspec.Mount{
			{Type: projectspec.Volume, Source: m.kubeResource(p), Subpath: "state", Target: "/cxz/state"},
			{Type: projectspec.Volume, Source: m.kubeResource(p), Subpath: "tools", Target: "/cxz/tools"},
			{Type: projectspec.Volume, Source: m.kubeResource(p), Subpath: "workspace", Target: p.RemoteWorkspace},
		},
		Workspace: p.RemoteWorkspace, HostWorkspace: p.RemoteWorkspace,
		Start: []string{kube.RuntimeBinary, "--state", projectspec.StateDir, "_project"},
	}
	// Where the runtime forwards its telemetry: the collector beside the
	// manager, across the network. See telemetry.ForwardSocket.
	if collector := os.Getenv("CXZ_TELEMETRY_COLLECTOR"); collector != "" {
		spec.Env["CXZ_TELEMETRY_COLLECTOR"] = collector
	}
	return spec, spec.Validate()
}

func (m *Manager) kubePodObject(p *Project, spec projectspec.Spec, image, user string, requests, limits map[string]string) map[string]any {
	name := m.kubeResource(p)
	env := []map[string]string{}
	for _, k := range spec.EnvKeys() {
		env = append(env, map[string]string{"name": k, "value": spec.Env[k]})
	}
	mounts := []map[string]any{}
	for _, mount := range spec.Mounts {
		mounts = append(mounts, map[string]any{"name": "project", "subPath": mount.Subpath, "mountPath": mount.Target})
	}
	mounts = append(mounts, map[string]any{"name": "secrets", "mountPath": "/cxz/secrets"}, map[string]any{"name": "assets", "mountPath": "/cxz/assets"})
	command := spec.Start
	if user != "" {
		command = append([]string{kube.RuntimeBinary, "_run-as", "--user", user, "--workdir", spec.Workspace, "--"}, spec.Start...)
	}
	// The second init container runs in the project's image, so it can see
	// that image's users; the cxz it runs is the one the first just copied.
	prepare := []string{"/mnt/tools/cxz", "_kube-prepare", "--runtime", "/cxz/runtime/runtime.json"}
	if user != "" {
		prepare = append(prepare, "--own", user, "/mnt/state/runtime.json", "/mnt/state", "/mnt/tools", "/mnt/workspace")
	} else {
		prepare = append(prepare, "/mnt/state/runtime.json")
	}
	volumes := []any{
		map[string]any{"name": "project", "persistentVolumeClaim": map[string]any{"claimName": name}},
		map[string]any{"name": "runtime", "secret": map[string]any{"secretName": name}},
		map[string]any{"name": "secrets", "emptyDir": map[string]any{"medium": "Memory"}},
		map[string]any{"name": "assets", "emptyDir": map[string]any{}},
	}
	var pullSecrets []any
	if secret := m.kubeSettings.Build.AuthSecret; secret != "" {
		// The registry credential: kubelet pulls built images with it, and a
		// build pushes with it (DOCKER_CONFIG=/cxz/registry).
		pullSecrets = append(pullSecrets, map[string]string{"name": secret})
		volumes = append(volumes, map[string]any{"name": "registry", "secret": map[string]any{"secretName": secret, "items": []any{map[string]string{"key": ".dockerconfigjson", "path": "config.json"}}}})
		mounts = append(mounts, map[string]any{"name": "registry", "mountPath": "/cxz/registry", "readOnly": true})
	}
	resources := map[string]any{}
	if len(requests) > 0 {
		resources["requests"] = requests
	}
	if len(limits) > 0 {
		resources["limits"] = limits
	}
	labels := spec.Labels()
	for k, v := range m.kubeLabels(p) {
		labels[k] = v
	}
	return map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		// The runtime's build and protocol, so a later manager can tell an
		// older runtime from its own (kubeRuntimeAge).
		"metadata": map[string]any{"name": name, "labels": labels, "annotations": map[string]string{
			annotationRevision: cxzupdate.Current().Revision,
			annotationProtocol: strconv.Itoa(cxzupdate.Protocol),
		}},
		"spec": map[string]any{
			// The pause container becomes PID 1 and reaps what agents leave.
			"shareProcessNamespace":         true,
			"restartPolicy":                 "Always",
			"terminationGracePeriodSeconds": 30,
			"automountServiceAccountToken":  false,
			"enableServiceLinks":            false,
			"initContainers": []any{
				map[string]any{
					"name": "tools", "image": m.kubeSettings.ToolsImage, "imagePullPolicy": m.kubeSettings.ImagePullPolicy,
					"command":      []string{"cxz", "_kube-prepare", "--copy-self", "/mnt/tools", "--copy", "/usr/local/bin/buildctl"},
					"volumeMounts": []any{map[string]any{"name": "project", "subPath": "tools", "mountPath": "/mnt/tools"}},
				},
				map[string]any{
					"name": "prepare", "image": image, "imagePullPolicy": m.kubeSettings.ImagePullPolicy,
					"command":         prepare,
					"securityContext": map[string]any{"runAsUser": 0},
					"volumeMounts": []any{
						map[string]any{"name": "project", "subPath": "tools", "mountPath": "/mnt/tools"},
						map[string]any{"name": "project", "subPath": "state", "mountPath": "/mnt/state"},
						map[string]any{"name": "project", "subPath": "workspace", "mountPath": "/mnt/workspace"},
						map[string]any{"name": "runtime", "mountPath": "/cxz/runtime", "readOnly": true},
					},
				},
			},
			"containers": []any{map[string]any{
				"name": "runtime", "image": image, "imagePullPolicy": m.kubeSettings.ImagePullPolicy,
				"command":         command,
				"workingDir":      spec.Workspace,
				"env":             env,
				"ports":           []any{map[string]any{"name": "runtime", "containerPort": projectspec.Port}},
				"readinessProbe":  map[string]any{"tcpSocket": map[string]any{"port": projectspec.Port}, "periodSeconds": 2, "failureThreshold": 3},
				"securityContext": map[string]any{"runAsUser": 0},
				"resources":       resources,
				"volumeMounts":    mounts,
			}},
			"volumes":          volumes,
			"imagePullSecrets": pullSecrets,
		},
	}
}

func (m *Manager) kubeWaitReady(ctx context.Context, p *Project) error {
	timeout, _ := time.ParseDuration(m.kubeSettings.ReadyTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		pod, err := m.kubePod(ctx, p)
		if err != nil {
			return err
		}
		if pod != nil {
			if pod.ready() {
				return nil
			}
			if problem := pod.problem(); problem != "" {
				return fmt.Errorf("project pod %s: %s", m.kubeResource(p), problem)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("project pod %s did not become ready within %s", m.kubeResource(p), timeout)
		case <-time.After(time.Second):
		}
	}
}

func (m *Manager) kubeDeletePod(ctx context.Context, p *Project) error {
	if err := m.kube.Delete(ctx, m.kube.Path("pods", m.kubeResource(p))); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		pod, err := m.kubePod(ctx, p)
		if err != nil || pod == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("project pod %s is still terminating", m.kubeResource(p))
		case <-time.After(time.Second):
		}
	}
}

// kubeRemove deletes everything the project owns in the cluster.
func (m *Manager) kubeRemove(ctx context.Context, p *Project) error {
	if err := m.kubeDeletePod(ctx, p); err != nil {
		return err
	}
	for _, resource := range []string{"persistentvolumeclaims", "secrets"} {
		if err := m.kube.Delete(ctx, m.kube.Path(resource, m.kubeResource(p))); err != nil {
			return err
		}
	}
	return nil
}

// kubeState is containerState for a pod: running only once the runtime is
// ready, absent when there is no pod, stopped otherwise.
type kubeObserved struct {
	state    string
	outdated bool
}

func (m *Manager) kubeStates(ctx context.Context) (map[string]kubeObserved, error) {
	var list struct {
		Items []kubePod `json:"items"`
	}
	if err := m.kube.List(ctx, m.kube.Path("pods"), projectspec.LabelOwner+"="+m.Owner, &list); err != nil {
		return nil, err
	}
	out := map[string]kubeObserved{}
	for i, pod := range list.Items {
		state := "stopped"
		if pod.ready() {
			state = "running"
		}
		out[pod.Metadata.Labels[projectspec.LabelProject]] = kubeObserved{state: state, outdated: m.kubeRuntimeAge(&list.Items[i]) != runtimeCurrent}
	}
	return out, nil
}

// kubeAddress is where the manager reaches the project's runtime.
func (m *Manager) kubeAddress(ctx context.Context, p *Project) (string, error) {
	pod, err := m.kubePod(ctx, p)
	if err != nil {
		return "", err
	}
	if pod == nil || !pod.ready() || pod.Status.PodIP == "" {
		return "", &Precondition{Message: "project is stopped; run cxz project up"}
	}
	return fmt.Sprintf("%s:%d", pod.Status.PodIP, projectspec.Port), nil
}

// containerCommand is `docker exec ARGS...` for the project's backend. On
// Kubernetes the same arguments are carried into the pod by cxz itself.
func (m *Manager) containerCommand(ctx context.Context, args ...string) *exec.Cmd {
	if m.kube == nil {
		return exec.CommandContext(ctx, "docker", append([]string{"exec"}, args...)...)
	}
	self, err := os.Executable()
	if err != nil {
		self = "cxz"
	}
	return exec.CommandContext(ctx, self, append([]string{"_kube-exec"}, KubeExecArgs(args)...)...)
}

// containerRun is dockerx.Run(ctx, "exec", args...) for the project's backend.
func (m *Manager) containerRun(ctx context.Context, args ...string) ([]byte, error) {
	if m.kube == nil {
		return dockerx.Run(ctx, append([]string{"exec"}, args...)...)
	}
	cmd := m.containerCommand(ctx, args...)
	var errout bytes.Buffer
	cmd.Stderr = &errout
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w: %.4000s", err, errout.String())
	}
	return b, nil
}

// KubeExecArgs translates docker exec's flags to _kube-exec's.
func KubeExecArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-i" || a == "--interactive":
			out = append(out, "-i")
		case a == "-t" || a == "--tty":
			out = append(out, "-t")
		case a == "-it" || a == "-ti":
			out = append(out, "-i", "-t")
		case (a == "--user" || a == "-u" || a == "--workdir" || a == "-w" || a == "--env" || a == "-e") && i+1 < len(args):
			flag := map[string]string{"-u": "--user", "-w": "--workdir", "-e": "--env"}[a]
			if flag == "" {
				flag = a
			}
			out = append(out, flag, args[i+1])
			i++
		default:
			// The container, then its command: nothing after is a flag.
			return append(out, args[i:]...)
		}
	}
	return out
}

// kubeProjects is Projects for pods: one list of the installation's pods
// answers for every project. A list that failed is unknown, not absent.
func (m *Manager) kubeProjects(ctx context.Context, all []*Project) (*api.ProjectList, error) {
	states, lookErr := m.kubeStates(ctx)
	out := &api.ProjectList{}
	for _, p := range all {
		state, name, outdated := "unknown", "", false
		if lookErr == nil {
			observed := states[p.storageID()]
			state, outdated = observed.state, observed.outdated
			if state == "" {
				state = "absent"
			} else {
				name = m.kubeResource(p)
			}
		}
		out.Projects = append(out.Projects, &api.Project{Id: p.ID, Workspace: p.Workspace, Name: p.Name, Config: p.Config, ContainerId: p.ContainerID, ContainerName: name, State: state, Error: p.Error, RemoteUser: p.RemoteUser, RemoteWorkspace: p.RemoteWorkspace, ProvisionState: p.Job.State, ProvisionStep: p.Job.Step, ProvisionAttempt: p.Job.Attempt, RuntimeOutdated: outdated})
	}
	return out, nil
}

// kubeStart is StartContainer for a pod. One verb whether the pod exists or
// not: what a person asked for is that the project runs.
func (m *Manager) kubeStart(ctx context.Context, r *api.ProjectRequest) (result *api.Receipt, retErr error) {
	name := r.Workspace
	if name == "" {
		return nil, fmt.Errorf("project name required")
	}
	p, err := m.resolve(ctx, name)
	if err != nil {
		if p, err = m.kubeRegister(ctx, name, r.Config); err != nil {
			return nil, err
		}
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	defer lock.Unlock()
	if p, err = m.resolve(ctx, p.ID); err != nil {
		return nil, err
	}
	defer m.recordFailure(ctx, p, &retErr)
	p.Job.Attempt++
	p.Job.State, p.Job.Step = "running", "pod"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	if err = m.kubeUp(ctx, p); err != nil {
		return nil, err
	}
	agent := r.Agent
	if agent == "" {
		agent = "claude"
	}
	// A pod fetches its agent CLI into its own volume, which the first time is
	// a download of a few hundred megabytes. The project runs without it, and
	// creating a session prepares it again under the same lock, so it is not
	// worth making the start wait.
	go m.kubePrepareAgent(p.ID, agent)
	p.Error = ""
	p.Job.State, p.Job.Step = "complete", "ready"
	p.Job.UpdatedAt = time.Now().UnixMilli()
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "started"}, nil
}

// errPodConversation is a session whose conversation is on its project's
// volume, which only the project's pod can read.
var errPodConversation = errors.New("the conversation is in the project's pod")

// kubeSession reaches the runtime that holds a session. The manager cannot
// read a pod's volume, so what it reads from a session's own store in Docker
// it asks the session's runtime for here; the project has to be running.
func (m *Manager) kubeSession(ctx context.Context, session string) (*grpc.ClientConn, api.SessionsClient, error) {
	p, err := m.sessionProject(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	return m.client(ctx, p)
}

func (m *Manager) kubePrepareAgent(id, agent string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	lock := m.projectLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := m.resolve(ctx, id)
	if err == nil {
		err = m.prepareAgent(ctx, p, agent)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "project %s: prepare %s CLI: %v\n", id, agent, err)
	}
}
