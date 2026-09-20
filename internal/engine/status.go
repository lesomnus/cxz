package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Info struct {
	Mode            string `json:"mode"`
	State           string `json:"state"`
	Health          string `json:"health,omitempty"`
	Image           string `json:"image"`
	ConfiguredImage string `json:"configured_image"`
	Endpoint        string `json:"endpoint"`
	BuildCache      string `json:"build_cache,omitempty"`
	Reclaimable     string `json:"reclaimable,omitempty"`
	UsageError      string `json:"usage_error,omitempty"`
}

func (e Engine) Info(ctx context.Context) (Info, error) {
	spec, err := e.Load()
	if err != nil {
		return Info{}, err
	}
	info := Info{Mode: spec.Mode, State: "not running", ConfiguredImage: spec.Image, Image: spec.Image, Endpoint: e.Endpoint()}
	v, err := e.inspect(ctx)
	if err != nil {
		return info, err
	}
	if v == nil {
		return info, nil
	}
	info.Image = v.Config.Image
	info.State = "stopped"
	if !v.State.Running {
		return info, nil
	}
	info.State = "running"
	info.Health = v.State.Health.Status
	data, err := e.run(ctx, "exec", v.ID, "docker", "--host", "unix:///var/run/docker.sock", "system", "df", "--format", "{{json .}}")
	if err != nil {
		info.UsageError = err.Error()
		return info, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		var row struct{ Type, Size, Reclaimable string }
		if json.Unmarshal([]byte(line), &row) == nil && row.Type == "Build Cache" {
			info.BuildCache = row.Size
			info.Reclaimable = row.Reclaimable
			break
		}
	}
	if info.BuildCache == "" {
		info.UsageError = "Build cache usage unavailable"
	}
	return info, nil
}
func (e Engine) PruneBuildCache(ctx context.Context) (string, error) {
	v, err := e.inspect(ctx)
	if err != nil {
		return "", err
	}
	if v == nil || !v.State.Running {
		return "", fmt.Errorf("shared Docker engine is not running")
	}
	// Explicit socket + verified container identity: never prune the host engine.
	b, err := e.run(ctx, "exec", v.ID, "docker", "--host", "unix:///var/run/docker.sock", "builder", "prune", "--all", "--force")
	if len(b) > 32768 {
		b = b[len(b)-32768:]
	}
	return strings.TrimSpace(string(b)), err
}
