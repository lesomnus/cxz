// Package engine manages the shared Docker-in-Docker engine owned by cxz.
package engine

import (
	"encoding/json"
	"fmt"
	"github.com/goccy/go-yaml"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Mode    string `json:"mode,omitempty"`
	Image   string `json:"image,omitempty"`
	Compose string `json:"compose,omitempty"`
}
type Spec struct {
	Mode     string          `json:"mode"`
	Image    string          `json:"image"`
	Override json.RawMessage `json:"override,omitempty"`
}

func (c Config) Validate() error {
	if c.Mode != "" && c.Mode != "off" && c.Mode != "dind" {
		return fmt.Errorf("docker.mode must be off or dind")
	}
	if strings.ContainsAny(c.Image, " $\t\r\n\x00") || strings.HasPrefix(c.Image, "-") {
		return fmt.Errorf("invalid Docker image")
	}
	return nil
}

// Snapshot reads only the explicitly selected override, relative to settings.json.
func (c Config) Snapshot(root string) (Spec, error) {
	if err := c.Validate(); err != nil {
		return Spec{}, err
	}
	s := Spec{Mode: c.Mode, Image: c.Image}
	if s.Mode == "" {
		s.Mode = "off"
	}
	if s.Image == "" {
		s.Image = "docker:29-dind"
	}
	if c.Compose != "" {
		path := c.Compose
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return s, err
			}
			path = filepath.Join(home, path[2:])
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return s, err
		}
		if len(b) > 256*1024 {
			return s, fmt.Errorf("Docker override exceeds 256 KiB")
		}
		var v map[string]any
		if err = yaml.Unmarshal(b, &v); err != nil {
			return s, err
		}
		s.Override, err = json.Marshal(v)
		if err != nil {
			return s, err
		}
	}
	return s, s.Validate()
}
func (s Spec) Validate() error {
	if err := (Config{Mode: s.Mode, Image: s.Image}).Validate(); err != nil {
		return err
	}
	if len(s.Override) > 256*1024 {
		return fmt.Errorf("Docker override exceeds 256 KiB")
	}
	if len(s.Override) == 0 {
		return nil
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(s.Override, &v); err != nil {
		return err
	}
	// cxz owns one engine service. Compose itself handles the service merge.
	for k := range v {
		if k != "services" {
			return fmt.Errorf("Docker override supports only services.dind; unexpected %s", k)
		}
	}
	var services map[string]map[string]json.RawMessage
	if err := json.Unmarshal(v["services"], &services); err != nil {
		return fmt.Errorf("Docker override requires services.dind")
	}
	if len(services) != 1 || services["dind"] == nil {
		return fmt.Errorf("Docker override requires exactly services.dind")
	}

	if strings.Contains(string(s.Override), "${") {
		return fmt.Errorf("Docker override values must be explicit; environment interpolation is not supported")
	}
	var mounts []json.RawMessage
	if raw, ok := services["dind"]["volumes"]; ok {
		if err := json.Unmarshal(raw, &mounts); err != nil {
			return fmt.Errorf("Docker volumes must be a list")
		}
		for _, raw := range mounts {
			var short string
			if json.Unmarshal(raw, &short) == nil {
				source, _, ok := strings.Cut(short, ":")
				if !ok || !filepath.IsAbs(source) {
					return fmt.Errorf("additional engine mounts must use absolute engine-host bind paths")
				}
			} else {
				var mount struct {
					Type   string
					Source string
				}
				if json.Unmarshal(raw, &mount) != nil || mount.Type != "bind" || !filepath.IsAbs(mount.Source) {
					return fmt.Errorf("additional engine mounts must use absolute engine-host bind paths")
				}
			}
		}
	}
	for k := range services["dind"] {
		switch k {
		case "image", "command", "environment", "volumes", "mem_limit", "cpus", "shm_size", "ulimits", "storage_opt", "dns", "extra_hosts", "cap_add", "cap_drop", "security_opt", "devices", "sysctls":
		default:
			return fmt.Errorf("unsupported managed Docker service option: %s", k)
		}
	}
	return nil
}
