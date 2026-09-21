// Package projectconfig snapshots user Compose overrides for project containers.
package projectconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
)

const MaxBytes = 256 * 1024
const ServiceVariable = "${DEVCONTAINER_SERVICE}"

type Config struct {
	Compose json.RawMessage `json:"compose,omitempty"`
}

// Spec contains file contents, never a client-local filename. Home substitution
// happens on the host CLI before this snapshot is sent to the manager.
type Spec struct {
	Compose json.RawMessage `json:"compose,omitempty"`
}

func (c Config) Validate() error {
	b := bytes.TrimSpace(c.Compose)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	var path string
	if json.Unmarshal(b, &path) == nil {
		if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
			return fmt.Errorf("devcontainer.compose path must not be empty or contain NUL")
		}
		return nil
	}
	return (Spec{Compose: b}).Validate()
}

func (c Config) Snapshot(root string) (Spec, error) {
	if err := c.Validate(); err != nil {
		return Spec{}, err
	}
	if len(c.Compose) == 0 || bytes.Equal(bytes.TrimSpace(c.Compose), []byte("null")) {
		return Spec{}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Spec{}, err
	}
	b := c.Compose
	var path string
	if json.Unmarshal(b, &path) == nil {
		path = strings.ReplaceAll(path, "${HOME}", home)
		if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		f, err := os.Open(path)
		if err != nil {
			return Spec{}, fmt.Errorf("devcontainer.compose: %w", err)
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return Spec{}, err
		}
		if !st.Mode().IsRegular() || st.Size() > MaxBytes {
			return Spec{}, fmt.Errorf("devcontainer.compose must be a regular file up to 256 KiB")
		}
		b, err = io.ReadAll(io.LimitReader(f, MaxBytes+1))
		if err != nil {
			return Spec{}, err
		}
	}
	if len(b) > MaxBytes {
		return Spec{}, fmt.Errorf("devcontainer.compose exceeds 256 KiB")
	}
	var v map[string]any
	d := yaml.NewDecoder(bytes.NewReader(b), yaml.Strict())
	if err := d.Decode(&v); err != nil {
		return Spec{}, fmt.Errorf("devcontainer.compose: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Spec{}, fmt.Errorf("devcontainer.compose must contain one YAML/JSON document")
	}
	// Walk decoded strings so quotes/backslashes in HOME cannot change the YAML
	// structure. Preserve Compose's $$ escape and all unrelated variables.
	var expand func(any) any
	expand = func(value any) any {
		switch x := value.(type) {
		case string:
			parts := strings.Split(x, "$$")
			for i := range parts {
				parts[i] = strings.ReplaceAll(parts[i], "${HOME}", strings.ReplaceAll(home, "$", "$$"))
			}
			return strings.Join(parts, "$$")
		case []any:
			for i := range x {
				x[i] = expand(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = expand(x[k])
			}
		}
		return value
	}
	b, err = json.Marshal(expand(v))
	if err != nil {
		return Spec{}, err
	}
	s := Spec{Compose: b}
	return s, s.Validate()
}

func (s Spec) Validate() error {
	if len(s.Compose) == 0 {
		return nil
	}
	if len(s.Compose) > MaxBytes {
		return fmt.Errorf("devcontainer.compose exceeds 256 KiB")
	}
	var v map[string]json.RawMessage
	if json.Unmarshal(s.Compose, &v) != nil || v == nil {
		return fmt.Errorf("devcontainer.compose must be a Compose object or file path")
	}
	raw, exists := v["services"]
	if !exists {
		return nil // Resource-only overrides (e.g. external volumes) are valid.
	}
	var services map[string]map[string]json.RawMessage
	if json.Unmarshal(raw, &services) != nil || services == nil {
		return fmt.Errorf("devcontainer.compose requires a services object")
	}
	for name, service := range services {
		if name == "" || service == nil {
			return fmt.Errorf("devcontainer.compose requires named service objects")
		}
	}
	return nil
}

// Render selects the devcontainer's service without changing the saved snapshot.
func (s Spec) Render(service string) ([]byte, error) {
	if err := s.Validate(); err != nil || len(s.Compose) == 0 {
		return nil, err
	}
	var v map[string]json.RawMessage
	_ = json.Unmarshal(s.Compose, &v)
	if _, exists := v["services"]; !exists {
		return json.Marshal(v)
	}
	var services map[string]json.RawMessage
	_ = json.Unmarshal(v["services"], &services)
	if body, ok := services[ServiceVariable]; ok {
		if service == "" || service == ServiceVariable {
			return nil, fmt.Errorf("devcontainer.compose requires a concrete devcontainer service")
		}
		if _, exists := services[service]; exists {
			return nil, fmt.Errorf("devcontainer.compose specifies both %s and %s", service, ServiceVariable)
		}
		delete(services, ServiceVariable)
		services[service] = body
		v["services"], _ = json.Marshal(services)
	}
	return json.Marshal(v)
}

func Load(root string) (Spec, error) {
	var s Spec
	b, err := os.ReadFile(filepath.Join(root, "devcontainer-settings.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, s.Validate()
}

func Save(root string, s Spec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return core.WriteFile(filepath.Join(root, "devcontainer-settings.json"), b)
}
