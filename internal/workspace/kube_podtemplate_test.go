package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// template decodes a template as the API would hand it over.
func template(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPodTemplates(t *testing.T) {
	templates := map[string]string{
		// The API requires an image; this one is ignored.
		"git": `{"template": {"spec": {
			"containers": [{"name": "runtime", "image": "-",
				"envFrom": [{"secretRef": {"name": "alice-git"}}],
				"volumeMounts": [{"name": "gh", "mountPath": "/home/vscode/.config/gh", "readOnly": true}]}],
			"volumes": [{"name": "gh", "secret": {"secretName": "alice-gh"}}],
			"tolerations": [{"key": "spot", "operator": "Exists"}]}}}`,
		"gpu": `{"template": {"metadata": {"annotations": {"team": "ml"}}, "spec": {
			"nodeSelector": {"gpu": "true"},
			"tolerations": [{"key": "nvidia.com/gpu", "operator": "Exists"}],
			"containers": [{"name": "runtime", "image": "-", "resources": {"limits": {"nvidia.com/gpu": "1"}}}]}}}`,
	}
	var asked []string
	m := kubeManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		asked = append(asked, r.URL.Path)
		body, ok := templates[name]
		if !ok {
			w.WriteHeader(404)
			w.Write([]byte(`{"kind": "Status", "reason": "NotFound", "message": "not found"}`))
			return
		}
		w.Write([]byte(body))
	}))
	m.kubeSettings.PodTemplates = []string{"git"}
	p := &Project{ID: "id", StorageID: "sid", RemoteWorkspace: "/workspaces/web", Kube: &KubeProject{
		PodTemplates: []string{"gpu"},
		PodTemplate:  template(t, `{"spec": {"containers": [{"name": "runtime", "env": [{"name": "NODE_ENV", "value": "test"}]}]}}`),
	}}
	spec, err := m.kubeSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := m.kubePodTemplates(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, " ") != "/api/v1/namespaces/ns/podtemplates/git /api/v1/namespaces/ns/podtemplates/gpu" {
		t.Fatal(asked)
	}
	pod, err := applyPodTemplates(m.kubePodObject(p, spec, "node:24", "vscode", map[string]string{"cpu": "1"}, nil), ts)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(pod)
	var got struct {
		Metadata struct {
			Labels      map[string]string
			Annotations map[string]string
		}
		Spec struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Tolerations  []map[string]string
			Containers   []struct {
				Name, Image  string
				Env          []map[string]string
				EnvFrom      []map[string]map[string]string `json:"envFrom"`
				VolumeMounts []map[string]any               `json:"volumeMounts"`
				Resources    map[string]map[string]string
			}
			Volumes []map[string]any
		}
	}
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	c := got.Spec.Containers[0]
	if len(got.Spec.Containers) != 1 || c.Image != "node:24" {
		t.Fatal("the runtime container was not merged into, or its image changed:", got.Spec.Containers)
	}
	if c.Resources["requests"]["cpu"] != "1" || c.Resources["limits"]["nvidia.com/gpu"] != "1" {
		t.Fatal(c.Resources)
	}
	if env := c.Env[len(c.Env)-1]; env["name"] != "NODE_ENV" || len(c.EnvFrom) != 1 || c.EnvFrom[0]["secretRef"]["name"] != "alice-git" {
		t.Fatal(c.Env, c.EnvFrom)
	}
	if last := c.VolumeMounts[len(c.VolumeMounts)-1]; last["mountPath"] != "/home/vscode/.config/gh" || len(c.VolumeMounts) != 6 {
		t.Fatal(c.VolumeMounts)
	}
	if v := got.Spec.Volumes[len(got.Spec.Volumes)-1]; v["name"] != "gh" || len(got.Spec.Volumes) != 5 {
		t.Fatal(got.Spec.Volumes)
	}
	if len(got.Spec.Tolerations) != 2 || got.Spec.NodeSelector["gpu"] != "true" || got.Metadata.Annotations["team"] != "ml" || got.Metadata.Labels["cxz.project"] != "sid" {
		t.Fatal(got.Spec.Tolerations, got.Spec.NodeSelector, got.Metadata)
	}

	p.Kube.PodTemplates = []string{"missing"}
	if _, err = m.kubePodTemplates(context.Background(), p); err == nil || !strings.Contains(err.Error(), `PodTemplate "missing" is not in this namespace`) {
		t.Fatal(err)
	}
}

func TestPodTemplateOwned(t *testing.T) {
	m := kubeManager(t, nil)
	p := &Project{ID: "id", StorageID: "sid", RemoteWorkspace: "/workspaces/web"}
	spec, err := m.kubeSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ template, refusal string }{
		"command":       {`{"spec": {"containers": [{"name": "runtime", "command": ["sleep"]}]}}`, "command is cxz's"},
		"args":          {`{"spec": {"containers": [{"name": "runtime", "args": ["x"]}]}}`, "args is cxz's"},
		"cxz env":       {`{"spec": {"containers": [{"name": "runtime", "env": [{"name": "CXZ_STATE", "value": "/tmp"}]}]}}`, "env CXZ_STATE is cxz's"},
		"cxz mount":     {`{"spec": {"containers": [{"name": "runtime", "volumeMounts": [{"name": "x", "mountPath": "/cxz/tools"}]}]}}`, "volumeMounts /cxz/tools is cxz's"},
		"under /cxz":    {`{"spec": {"containers": [{"name": "runtime", "volumeMounts": [{"name": "x", "mountPath": "/cxz/extra"}]}]}}`, "/cxz is cxz's"},
		"workspace":     {`{"spec": {"containers": [{"name": "runtime", "volumeMounts": [{"name": "x", "mountPath": "/workspaces/web"}]}]}}`, "is cxz's"},
		"volume":        {`{"spec": {"volumes": [{"name": "project", "emptyDir": {}}]}}`, "volume project is cxz's"},
		"init":          {`{"spec": {"initContainers": [{"name": "tools", "image": "evil"}]}}`, "initContainer tools is cxz's"},
		"label":         {`{"metadata": {"labels": {"cxz.owner": "someone"}}}`, "label cxz.owner is cxz's"},
		"non-root":      {`{"spec": {"containers": [{"name": "runtime", "securityContext": {"runAsNonRoot": true}}]}}`, "starts as root"},
		"restart":       {`{"spec": {"restartPolicy": "Never"}}`, "spec.restartPolicy is cxz's"},
		"not a pod tpl": {`{"kind": "Pod"}`, `has "kind"`},
	} {
		_, err := applyPodTemplates(m.kubePodObject(p, spec, "node:24", "vscode", nil, nil), []podTemplate{{source: "PodTemplate t", body: template(t, c.template)}})
		if err == nil || !strings.Contains(err.Error(), c.refusal) || !strings.HasPrefix(err.Error(), "PodTemplate t: ") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A sidecar, an init container of its own and a security context that
	// leaves the user alone are the template's to add.
	_, err = applyPodTemplates(m.kubePodObject(p, spec, "node:24", "vscode", nil, nil), []podTemplate{{source: "t", body: template(t, `{"spec": {
		"initContainers": [{"name": "fetch", "image": "busybox"}],
		"containers": [{"name": "proxy", "image": "envoy"}, {"name": "runtime", "securityContext": {"capabilities": {"add": ["SYS_PTRACE"]}}}],
		"serviceAccountName": "builder"}}`)}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKubeRegisterPodTemplate(t *testing.T) {
	m := kubeManager(t, nil)
	if _, err := m.kubeRegister(context.Background(), "web", `{"pod_templates": ["git"], "pod_template": {"spec": {"nodeSelector": {"a": "b"}}}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.kubeRegister(context.Background(), "bad", `{"pod_template": {"apiVersion": "v1"}}`); err == nil {
		t.Fatal("a pod_template that is not a template was accepted")
	}
}
