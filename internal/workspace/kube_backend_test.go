package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/kube"
)

func TestKubeExecArgs(t *testing.T) {
	got := KubeExecArgs([]string{"-it", "--user", "vscode", "-w", "/w", "-e", "TERM=x", "pod", "sh", "-c", "echo -i"})
	want := []string{"-i", "-t", "--user", "vscode", "--workdir", "/w", "--env", "TERM=x", "pod", "sh", "-c", "echo -i"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func kubeManager(t *testing.T, handler http.Handler) *Manager {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db, Root: t.TempDir(), Owner: strings.Repeat("o", 24)}
	m.kubeSettings = KubeSettings{Image: "base", User: "vscode", Storage: "20Gi", ToolsImage: "cxz:test", ImagePullPolicy: "IfNotPresent", ReadyTimeout: "1s", Requests: map[string]string{"cpu": "1"}}
	if handler != nil {
		srv := httptest.NewTLSServer(handler)
		t.Cleanup(srv.Close)
		ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
		if m.kube, err = kube.New(kube.Config{Server: srv.URL, CA: ca, Namespace: "ns", Token: "t"}); err != nil {
			t.Fatal(err)
		}
	} else if m.kube, err = kube.New(kube.Config{Server: "https://unused", Namespace: "ns"}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestKubeRegister(t *testing.T) {
	m := kubeManager(t, nil)
	ctx := context.Background()
	for _, bad := range []string{"Demo", "has space", "-x", strings.Repeat("a", 41), "/workspaces/x"} {
		if _, err := m.kubeRegister(ctx, bad, ""); err == nil {
			t.Errorf("%q accepted as a project name", bad)
		}
	}
	p, err := m.kubeRegister(ctx, "web", `{"image":"node:24","storage":"5Gi","limits":{"memory":"2Gi"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Workspace != "/workspaces/web" || p.RemoteWorkspace != "/workspaces/web" || p.Kube.Image != "node:24" || p.Kube.Storage != "5Gi" || p.Token == "" {
		t.Fatalf("%+v", p)
	}
	again, err := m.kubeRegister(ctx, "web", "")
	if err != nil || again.ID != p.ID {
		t.Fatal("registering a name twice made another project", err)
	}
	q, err := m.kubeRegister(ctx, "api", "golang:1.27")
	if err != nil || q.Kube.Image != "golang:1.27" {
		t.Fatal(q, err)
	}
	y, err := m.kubeRegister(ctx, "yaml", `---
image: node:24
user: hypnos
requests: { cpu: "1" }
pod_templates: [gpu]
pod_template:
  spec:
    nodeSelector:
      disk: ssd
`)
	if err != nil {
		t.Fatal(err)
	}
	if y.Kube.Image != "node:24" || y.Kube.User != "hypnos" || y.Kube.Requests["cpu"] != "1" ||
		!reflect.DeepEqual(y.Kube.PodTemplates, []string{"gpu"}) ||
		!reflect.DeepEqual(y.Kube.PodTemplate, map[string]any{
			"spec": map[string]any{"nodeSelector": map[string]any{"disk": "ssd"}},
		}) {
		t.Fatalf("unexpected YAML settings: %+v", y.Kube)
	}
	for _, bad := range []string{
		"image: [",
		"image: node:24\n---\nimage: alpine",
		"imgae: node:24",
		"requests: {cpu: 1}",
		"pod_template: {kind: PodTemplate}",
	} {
		if _, err := m.kubeRegister(ctx, "invalid", bad); err == nil {
			t.Errorf("accepted invalid settings %q", bad)
		}
	}
	if image, user, storage, requests, limits := m.kubeProject(p); image != "node:24" || user != "vscode" || storage != "5Gi" || requests["cpu"] != "1" || limits["memory"] != "2Gi" {
		t.Fatal(image, user, storage, requests, limits)
	}
}

func TestKubePodObject(t *testing.T) {
	m := kubeManager(t, nil)
	p := &Project{ID: "id", StorageID: "sid", RemoteWorkspace: "/workspaces/web"}
	spec, err := m.kubeSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m.kubePodObject(p, spec, "node:24", "vscode", map[string]string{"cpu": "1"}, nil))
	var pod struct {
		Metadata struct {
			Name   string
			Labels map[string]string
		}
		Spec struct {
			ShareProcessNamespace bool `json:"shareProcessNamespace"`
			InitContainers        []struct {
				Name, Image string
				Command     []string
			} `json:"initContainers"`
			Containers []struct {
				Image        string
				Command      []string
				WorkingDir   string `json:"workingDir"`
				Env          []map[string]string
				VolumeMounts []map[string]any `json:"volumeMounts"`
				Resources    map[string]map[string]string
			}
			Volumes []map[string]any
		}
	}
	if err = json.Unmarshal(b, &pod); err != nil {
		t.Fatal(err)
	}
	if pod.Metadata.Name != "cxz-oooooooooooo-sid" || pod.Metadata.Labels["cxz.owner"] != m.Owner || pod.Metadata.Labels["cxz.project"] != "sid" || !pod.Spec.ShareProcessNamespace {
		t.Fatalf("%+v", pod.Metadata)
	}
	tools, prepare := pod.Spec.InitContainers[0], pod.Spec.InitContainers[1]
	if tools.Image != "cxz:test" || strings.Join(tools.Command, " ") != "cxz _kube-prepare --copy-self /mnt/tools --copy /usr/local/bin/buildctl" {
		t.Fatal("tools", tools)
	}
	if prepare.Image != "node:24" || strings.Join(prepare.Command, " ") != "/mnt/tools/cxz _kube-prepare --runtime /cxz/runtime/runtime.json --own vscode /mnt/state/runtime.json /mnt/state /mnt/tools /mnt/workspace" {
		t.Fatal("prepare", prepare)
	}
	main := pod.Spec.Containers[0]
	if main.Image != "node:24" || strings.Join(main.Command, " ") != "/cxz/tools/cxz _run-as --user vscode --workdir /workspaces/web -- /cxz/tools/cxz --state /cxz/state/data _project" || main.WorkingDir != "/workspaces/web" || main.Resources["requests"]["cpu"] != "1" {
		t.Fatal("runtime", main)
	}
	mounts := map[string]string{}
	for _, v := range main.VolumeMounts {
		sub, _ := v["subPath"].(string)
		mounts[v["mountPath"].(string)] = v["name"].(string) + ":" + sub
	}
	want := map[string]string{"/cxz/state": "project:state", "/cxz/tools": "project:tools", "/workspaces/web": "project:workspace", "/cxz/secrets": "secrets:", "/cxz/assets": "assets:"}
	if !reflect.DeepEqual(mounts, want) {
		t.Fatal(mounts)
	}
	env := map[string]string{}
	for _, e := range main.Env {
		env[e["name"]] = e["value"]
	}
	if env["CXZ_STATE"] != "/cxz/state/data" || env["CXZ_PROJECT_ID"] != "id" {
		t.Fatal(env)
	}
	// Without a user the image's own runs it, and nothing is handed over.
	b, _ = json.Marshal(m.kubePodObject(p, spec, "node:24", "", nil, nil))
	json.Unmarshal(b, &pod)
	if strings.Join(pod.Spec.Containers[0].Command, " ") != "/cxz/tools/cxz --state /cxz/state/data _project" || strings.Contains(strings.Join(pod.Spec.InitContainers[1].Command, " "), "--own") {
		t.Fatal(pod.Spec.Containers[0].Command, pod.Spec.InitContainers[1].Command)
	}
}

func TestKubeProjectsAndOwnership(t *testing.T) {
	failList := false
	pods := []map[string]any{
		{"metadata": map[string]any{"name": "cxz-oooooooooooo-run", "labels": map[string]string{"cxz.owner": strings.Repeat("o", 24), "cxz.project": "run"}}, "status": map[string]any{"phase": "Running", "podIP": "10.0.0.9", "containerStatuses": []any{map[string]any{"name": "runtime", "ready": true}}}},
		{"metadata": map[string]any{"name": "cxz-oooooooooooo-boot", "labels": map[string]string{"cxz.owner": strings.Repeat("o", 24), "cxz.project": "boot"}}, "status": map[string]any{"phase": "Pending"}},
		{"metadata": map[string]any{"name": "cxz-oooooooooooo-theirs", "labels": map[string]string{"cxz.owner": "someone-else", "cxz.project": "theirs"}}, "status": map[string]any{"phase": "Running", "podIP": "10.0.0.8", "containerStatuses": []any{map[string]any{"name": "runtime", "ready": true}}}},
	}
	m := kubeManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pods") {
			if failList {
				http.Error(w, "unavailable", 503)
				return
			}
			if r.URL.Query().Get("labelSelector") != "cxz.owner="+strings.Repeat("o", 24) {
				http.Error(w, "unfiltered list", 400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"items": pods[:2]})
			return
		}
		for _, pod := range pods {
			if strings.HasSuffix(r.URL.Path, "/"+pod["metadata"].(map[string]any)["name"].(string)) {
				json.NewEncoder(w).Encode(pod)
				return
			}
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"reason":"NotFound","message":"not found"}`))
	}))
	ctx := context.Background()
	all := []*Project{{ID: "1", StorageID: "run"}, {ID: "2", StorageID: "boot"}, {ID: "3", StorageID: "none"}}
	list, err := m.kubeProjects(ctx, all)
	if err != nil {
		t.Fatal(err)
	}
	states := []string{list.Projects[0].State, list.Projects[1].State, list.Projects[2].State}
	if strings.Join(states, ",") != "running,stopped,absent" {
		t.Fatal(states)
	}
	if addr, err := m.kubeAddress(ctx, all[0]); err != nil || addr != "10.0.0.9:7348" {
		t.Fatal(addr, err)
	}
	if _, err = m.kubeAddress(ctx, all[1]); err == nil {
		t.Fatal("a pod that is not ready was given an address")
	}
	if _, err = m.kubeAddress(ctx, &Project{ID: "4", StorageID: "theirs"}); err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatal("a pod with another owner's labels was used", err)
	}
	failList = true
	if list, err = m.kubeProjects(ctx, all); err != nil || list.Projects[0].State != "unknown" {
		t.Fatal("a failed look was reported as an observation", list, err)
	}
}

func TestKubeBuildSettings(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "..", "../x", "a/../../x"} {
		if _, err := workspacePath(bad); err == nil {
			t.Errorf("%q accepted as a workspace path", bad)
		}
	}
	if p, err := workspacePath("./.devcontainer//Dockerfile"); err != nil || p != ".devcontainer/Dockerfile" {
		t.Fatal(p, err)
	}
	ctx := context.Background()
	docker := &Manager{}
	if _, err := docker.BuildProject(ctx, &api.BuildProjectInput{Workspace: "x"}); err == nil || !strings.Contains(err.Error(), "Kubernetes") {
		t.Fatal("a Docker manager offered to build", err)
	}
	m := kubeManager(t, nil)
	if _, err := m.BuildProject(ctx, &api.BuildProjectInput{Workspace: "x"}); err == nil || !strings.Contains(err.Error(), "build.buildkit") {
		t.Fatal("building without a builder was not explained", err)
	}
	p := &Project{ID: "id", StorageID: "sid", RemoteWorkspace: "/workspaces/web", Kube: &KubeProject{Image: "node:24", Built: "registry/x/web@sha256:abc"}}
	if image, _, _, _, _ := m.kubeProject(p); image != "registry/x/web@sha256:abc" {
		t.Fatal("a built image did not take precedence", image)
	}
	m.kubeSettings.Build = KubeBuild{BuildKit: "tcp://b:1234", Registry: "r/x", AuthSecret: "pull"}
	spec, err := m.kubeSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m.kubePodObject(p, spec, "img", "vscode", nil, nil))
	var pod struct {
		Spec struct {
			ImagePullSecrets []map[string]string `json:"imagePullSecrets"`
			Containers       []struct {
				VolumeMounts []map[string]any `json:"volumeMounts"`
			}
			Volumes []map[string]any
		}
	}
	json.Unmarshal(b, &pod)
	if len(pod.Spec.ImagePullSecrets) != 1 || pod.Spec.ImagePullSecrets[0]["name"] != "pull" {
		t.Fatal("pods do not pull with the registry credential", pod.Spec.ImagePullSecrets)
	}
	found := false
	for _, v := range pod.Spec.Containers[0].VolumeMounts {
		if v["mountPath"] == "/cxz/registry" && v["readOnly"] == true {
			found = true
		}
	}
	if !found {
		t.Fatal("the registry credential is not where a build looks for it")
	}
}
