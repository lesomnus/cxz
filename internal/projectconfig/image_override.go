package projectconfig

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The shared Compose override is written as Compose because most devcontainers
// are, but it promises to apply to every project in the installation -- and the
// projects most in need of it are the ones with no devcontainer of their own,
// which cxz gives an image-based default. Those never invoke Compose, so the
// override has to be translated rather than handed to it.
//
// Only what has a faithful equivalent is translated: bind and volume mounts,
// and environment. Anything else is refused by name instead of dropped, because
// an override that silently did less than it says would be worse than one that
// does not start.
type ImageOverride struct {
	Mounts []string          `json:"mounts,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

func (o ImageOverride) Empty() bool { return len(o.Mounts) == 0 && len(o.Env) == 0 }

// RenderImage translates the override for a devcontainer that does not use
// Compose. The service may be named or left as ${DEVCONTAINER_SERVICE}: without
// Compose there is only one container, so either way it is this one.
func (s Spec) RenderImage() (ImageOverride, error) {
	var out ImageOverride
	if err := s.Validate(); err != nil || len(s.Compose) == 0 {
		return out, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(s.Compose, &root); err != nil {
		return out, err
	}
	for _, key := range sortedKeys(root) {
		switch key {
		case "services", "volumes", "name":
			// volumes only declares names; a volume mount creates them anyway.
		default:
			return out, fmt.Errorf("devcontainer.compose %s cannot apply to an image or Dockerfile devcontainer; give the project a Compose devcontainer or remove it from the shared override", key)
		}
	}
	raw, exists := root["services"]
	if !exists {
		return out, nil
	}
	var services map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &services); err != nil {
		return out, err
	}
	if len(services) == 0 {
		return out, nil
	}
	if len(services) > 1 {
		return out, fmt.Errorf("devcontainer.compose defines %d services; an image or Dockerfile devcontainer has only one container, so sidecars need a Compose devcontainer", len(services))
	}
	for _, service := range services {
		for _, key := range sortedKeys(service) {
			switch key {
			case "volumes":
				mounts, err := imageMounts(service[key])
				if err != nil {
					return out, err
				}
				out.Mounts = mounts
			case "environment":
				env, err := imageEnv(service[key])
				if err != nil {
					return out, err
				}
				out.Env = env
			default:
				return out, fmt.Errorf("devcontainer.compose service key %q cannot apply to an image or Dockerfile devcontainer; only volumes and environment translate", key)
			}
		}
	}
	return out, nil
}

// imageMounts accepts both Compose spellings. The long form maps across
// directly; the short "source:target[:ro]" form is expanded here rather than
// passed through, because a devcontainer mount string is not Compose syntax.
func imageMounts(raw json.RawMessage) ([]string, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("devcontainer.compose volumes must be a list")
	}
	var out []string
	for _, entry := range entries {
		var short string
		if json.Unmarshal(entry, &short) == nil {
			mount, err := shortMount(short)
			if err != nil {
				return nil, err
			}
			out = append(out, mount)
			continue
		}
		var long struct {
			Type     string `json:"type"`
			Source   string `json:"source"`
			Target   string `json:"target"`
			ReadOnly bool   `json:"read_only"`
		}
		if err := json.Unmarshal(entry, &long); err != nil {
			return nil, fmt.Errorf("devcontainer.compose volume must be a string or an object")
		}
		if long.Target == "" || long.Source == "" {
			return nil, fmt.Errorf("devcontainer.compose volume requires source and target")
		}
		if long.Type == "" {
			long.Type = "bind"
		}
		if long.Type != "bind" && long.Type != "volume" {
			return nil, fmt.Errorf("devcontainer.compose volume type %q cannot apply to an image or Dockerfile devcontainer", long.Type)
		}
		mount := "type=" + long.Type + ",source=" + long.Source + ",target=" + long.Target
		if long.ReadOnly {
			mount += ",readonly"
		}
		out = append(out, mount)
	}
	return out, nil
}

func shortMount(v string) (string, error) {
	parts := strings.Split(v, ":")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("devcontainer.compose volume %q must be source:target[:ro]", v)
	}
	kind := "bind"
	if !strings.HasPrefix(parts[0], "/") && !strings.HasPrefix(parts[0], ".") && !strings.HasPrefix(parts[0], "~") {
		kind = "volume"
	}
	mount := "type=" + kind + ",source=" + parts[0] + ",target=" + parts[1]
	if len(parts) == 3 {
		if parts[2] != "ro" {
			return "", fmt.Errorf("devcontainer.compose volume option %q is not supported; use ro", parts[2])
		}
		mount += ",readonly"
	}
	return mount, nil
}

func imageEnv(raw json.RawMessage) (map[string]string, error) {
	out := map[string]string{}
	var mapping map[string]string
	if json.Unmarshal(raw, &mapping) == nil {
		return mapping, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("devcontainer.compose environment must be a mapping or a list")
	}
	for _, entry := range list {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("devcontainer.compose environment %q must be NAME=value", entry)
		}
		out[name] = value
	}
	return out, nil
}

// Sorting keeps the refusal message stable, so the same override always names
// the same unsupported key rather than whichever the map happened to yield.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
