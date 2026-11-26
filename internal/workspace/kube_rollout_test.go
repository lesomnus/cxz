package workspace

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/lesomnus/cxz/internal/cxzupdate"
)

func TestKubeRuntimeAge(t *testing.T) {
	m := kubeManager(t, nil)
	p := &Project{ID: "id", StorageID: "sid", RemoteWorkspace: "/workspaces/web"}
	spec, err := m.kubeSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	// The pod cxz makes says which runtime it carries, and is current.
	b, _ := json.Marshal(m.kubePodObject(p, spec, "node:24", "vscode", nil, nil))
	var pod kubePod
	if err = json.Unmarshal(b, &pod); err != nil {
		t.Fatal(err)
	}
	if pod.Metadata.Annotations[annotationProtocol] != strconv.Itoa(cxzupdate.Protocol) || pod.Metadata.Annotations[annotationRevision] != cxzupdate.Current().Revision {
		t.Fatal(pod.Metadata.Annotations)
	}
	if pod.Metadata.Labels["app.kubernetes.io/managed-by"] != "cxz" || pod.Metadata.Labels["cxz.project"] != "sid" {
		t.Fatal(pod.Metadata.Labels)
	}
	if m.kubeRuntimeAge(&pod) != runtimeCurrent {
		t.Fatal("a pod this manager made is outdated")
	}
	// The manager's image changed under it: the pod is an older runtime.
	m.kubeSettings.ToolsImage = "cxz:newer"
	if m.kubeRuntimeAge(&pod) != runtimeOutdated {
		t.Fatal("an older runtime was not noticed")
	}
	pod.Metadata.Annotations[annotationProtocol] = strconv.Itoa(cxzupdate.Protocol + 1)
	if m.kubeRuntimeAge(&pod) != runtimeIncompatible {
		t.Fatal("another protocol was not noticed")
	}
	// A pod from before the annotation speaks this protocol.
	delete(pod.Metadata.Annotations, annotationProtocol)
	if m.kubeRuntimeAge(&pod) != runtimeOutdated {
		t.Fatal("a pod without the annotation was judged by it")
	}
}

func TestKubeRuntimeRolloutSetting(t *testing.T) {
	for value, want := range map[string]string{"": rolloutIdle, `"idle"`: rolloutIdle, `"manual"`: rolloutManual, `"always"`: ""} {
		raw := `{}`
		if value != "" {
			raw = `{"runtime_rollout": ` + value + `}`
		}
		t.Setenv("CXZ_KUBERNETES", raw)
		s, err := loadKubeSettings()
		if want == "" {
			if err == nil {
				t.Errorf("%s accepted", value)
			}
			continue
		}
		if err != nil || s.RuntimeRollout != want {
			t.Errorf("%s: %q %v", value, s.RuntimeRollout, err)
		}
	}
}
