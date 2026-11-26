package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/lesomnus/cxz/internal/kube"
)

// A project pod is cxz's pod with the installation's and the project's
// PodTemplates merged over it, in order: the settings' pod_templates, the
// project's pod_templates, then the project's inline pod_template. They add
// what cxz has no feature for -- a credential mounted from a Secret, a node
// selector, a GPU -- and may not change what cxz runs the project with.

// runtimeContainer is the project's own container in cxz's pod, and the name
// a template uses to add to it.
const runtimeContainer = "runtime"

// mergeKeys are the lists merged item by item, by this field, as kubectl's
// strategic merge does. Any other list is replaced, except appendLists.
var mergeKeys = map[string]string{
	"containers":          "name",
	"initContainers":      "name",
	"ephemeralContainers": "name",
	"volumes":             "name",
	"env":                 "name",
	"imagePullSecrets":    "name",
	"volumeMounts":        "mountPath",
	"volumeDevices":       "devicePath",
	"ports":               "containerPort",
	"hostAliases":         "ip",
}

// appendLists add to what is there, so templates compose: a GPU template's
// toleration does not drop a spot-node template's.
var appendLists = map[string]bool{"tolerations": true, "envFrom": true, "topologySpreadConstraints": true}

type podTemplate struct {
	source string
	body   map[string]any
}

// kubePodTemplates are the templates for p's pod, read from the namespace now:
// a changed PodTemplate applies the next time the pod is created.
func (m *Manager) kubePodTemplates(ctx context.Context, p *Project) ([]podTemplate, error) {
	names := append([]string(nil), m.kubeSettings.PodTemplates...)
	var inline map[string]any
	if p.Kube != nil {
		names = append(names, p.Kube.PodTemplates...)
		inline = p.Kube.PodTemplate
	}
	var out []podTemplate
	for _, name := range names {
		var obj struct {
			Template map[string]any `json:"template"`
		}
		if err := m.kube.Get(ctx, m.kube.Path("podtemplates", name), &obj); kube.IsNotFound(err) {
			return nil, fmt.Errorf("PodTemplate %q is not in this namespace", name)
		} else if err != nil {
			return nil, fmt.Errorf("PodTemplate %q: %w", name, err)
		}
		out = append(out, podTemplate{source: "PodTemplate " + name, body: obj.Template})
	}
	if len(inline) > 0 {
		out = append(out, podTemplate{source: "the project's pod_template", body: inline})
	}
	return out, nil
}

// validPodTemplate is the shape of a PodTemplate's template: metadata and spec.
func validPodTemplate(t map[string]any) error {
	for k, v := range t {
		if k != "metadata" && k != "spec" {
			return fmt.Errorf("pod_template has %q; it takes metadata and spec", k)
		}
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("pod_template %s is not an object", k)
		}
	}
	return nil
}

// applyPodTemplates merges the templates over cxz's pod and refuses one that
// changes what cxz owns, naming it.
func applyPodTemplates(pod map[string]any, templates []podTemplate) (map[string]any, error) {
	if len(templates) == 0 {
		return pod, nil
	}
	base, err := normalized(pod)
	if err != nil {
		return nil, err
	}
	merged, _ := normalized(base)
	for _, t := range templates {
		body, err := normalized(t.body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.source, err)
		}
		tm, _ := body.(map[string]any)
		if err = validPodTemplate(tm); err != nil {
			return nil, fmt.Errorf("%s: %w", t.source, err)
		}
		// The API requires an image on every container of a PodTemplate; the
		// project's image is chosen by its settings and builds, never here.
		if spec, ok := tm["spec"].(map[string]any); ok {
			if c := named(spec["containers"], "name", runtimeContainer); c != nil {
				delete(c, "image")
			}
		}
		merged = merge(merged, tm, "")
		if err = podOwned(base.(map[string]any), merged.(map[string]any)); err != nil {
			return nil, fmt.Errorf("%s: %w", t.source, err)
		}
	}
	return merged.(map[string]any), nil
}

// normalized is v as JSON would decode it, so cxz's typed values and a
// template's compare alike.
func normalized(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	return out, json.Unmarshal(b, &out)
}

func merge(dst, src any, field string) any {
	switch s := src.(type) {
	case map[string]any:
		d, ok := dst.(map[string]any)
		if !ok {
			return s
		}
		for k, v := range s {
			d[k] = merge(d[k], v, k)
		}
		return d
	case []any:
		d, _ := dst.([]any)
		if appendLists[field] {
			return append(d, s...)
		}
		key, ok := mergeKeys[field]
		if !ok {
			return s
		}
		for _, item := range s {
			im, ok := item.(map[string]any)
			if !ok {
				d = append(d, item)
				continue
			}
			if existing := named(d, key, im[key]); existing != nil {
				merge(existing, im, "")
			} else {
				d = append(d, im)
			}
		}
		return d
	default:
		return src
	}
}

// named is the item of a list whose key field is value.
func named(list any, key string, value any) map[string]any {
	items, _ := list.([]any)
	for _, item := range items {
		if m, ok := item.(map[string]any); ok && fmt.Sprint(m[key]) == fmt.Sprint(value) {
			return m
		}
	}
	return nil
}

// podOwned refuses a merged pod that differs from cxz's where cxz depends on
// it: how the runtime starts and is reached, its volumes, its init
// containers, its identity, and everything under /cxz.
func podOwned(base, merged map[string]any) error {
	bm, mm := obj(base, "metadata"), obj(merged, "metadata")
	if !reflect.DeepEqual(bm["name"], mm["name"]) || mm["namespace"] != nil {
		return fmt.Errorf("the pod's name and namespace are cxz's")
	}
	bl, ml := obj(bm, "labels"), obj(mm, "labels")
	for k, v := range ml {
		if strings.HasPrefix(k, "cxz.") && !reflect.DeepEqual(bl[k], v) {
			return fmt.Errorf("label %s is cxz's", k)
		}
	}
	bs, ms := obj(base, "spec"), obj(merged, "spec")
	for _, k := range []string{"shareProcessNamespace", "restartPolicy"} {
		if !reflect.DeepEqual(bs[k], ms[k]) {
			return fmt.Errorf("spec.%s is cxz's", k)
		}
	}
	for _, list := range []string{"initContainers", "volumes"} {
		items, _ := bs[list].([]any)
		for _, item := range items {
			name := item.(map[string]any)["name"]
			if !reflect.DeepEqual(item, named(ms[list], "name", name)) {
				return fmt.Errorf("%s %v is cxz's", strings.TrimSuffix(list, "s"), name)
			}
		}
	}
	br, mr := named(bs["containers"], "name", runtimeContainer), named(ms["containers"], "name", runtimeContainer)
	for _, k := range []string{"image", "command", "args", "workingDir", "ports", "readinessProbe", "livenessProbe", "startupProbe", "lifecycle"} {
		if !reflect.DeepEqual(br[k], mr[k]) {
			return fmt.Errorf("the runtime container's %s is cxz's", k)
		}
	}
	bsc, msc := obj(br, "securityContext"), obj(mr, "securityContext")
	if !reflect.DeepEqual(bsc["runAsUser"], msc["runAsUser"]) || msc["runAsNonRoot"] == true {
		return fmt.Errorf("the runtime container starts as root and drops to the project's user itself; leave runAsUser and runAsNonRoot")
	}
	for _, list := range []struct{ field, key string }{{"env", "name"}, {"volumeMounts", "mountPath"}} {
		items, _ := br[list.field].([]any)
		for _, item := range items {
			value := item.(map[string]any)[list.key]
			if !reflect.DeepEqual(item, named(mr[list.field], list.key, value)) {
				return fmt.Errorf("the runtime container's %s %v is cxz's", list.field, value)
			}
		}
	}
	mounts, _ := mr["volumeMounts"].([]any)
	for _, item := range mounts {
		path, _ := item.(map[string]any)["mountPath"].(string)
		if (path == "/cxz" || strings.HasPrefix(path, "/cxz/")) && named(br["volumeMounts"], "mountPath", path) == nil {
			return fmt.Errorf("%s: /cxz is cxz's", path)
		}
	}
	return nil
}

func obj(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	return v
}
