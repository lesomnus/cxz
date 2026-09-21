// Package projectconfig snapshots user Compose overrides for project containers.
package projectconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
)

const MaxBytes = 256 * 1024
const Filename = "docker-compose.yaml"
const ServiceVariable = "${DEVCONTAINER_SERVICE}"

type Config struct {
	Compose string `json:"compose,omitempty"`
	// Read old inline settings only so the editor can migrate them without loss.
	LegacyInline json.RawMessage `json:"-"`
}

func (c *Config) UnmarshalJSON(b []byte) error {
	var raw struct {
		Compose json.RawMessage `json:"compose"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&raw); err != nil {
		return err
	}
	*c = Config{}
	if len(raw.Compose) == 0 || bytes.Equal(bytes.TrimSpace(raw.Compose), []byte("null")) {
		return nil
	}
	if json.Unmarshal(raw.Compose, &c.Compose) == nil {
		return c.Validate()
	}
	if err := (Spec{Compose: raw.Compose}).Validate(); err != nil {
		return err
	}
	c.LegacyInline = bytes.Clone(raw.Compose)
	return nil
}

func (c Config) MarshalJSON() ([]byte, error) {
	if len(c.LegacyInline) > 0 {
		return json.Marshal(struct {
			Compose json.RawMessage `json:"compose"`
		}{c.LegacyInline})
	}
	return json.Marshal(struct {
		Compose string `json:"compose,omitempty"`
	}{c.Compose})
}

// Spec stores the resolved override, including included Compose files. It never
// depends on a client-local filename after publication to the manager.
type Spec struct {
	Compose json.RawMessage `json:"compose,omitempty"`
}

func (c Config) Validate() error {
	if c.Compose != "" && (strings.TrimSpace(c.Compose) == "" || strings.ContainsRune(c.Compose, 0)) {
		return fmt.Errorf("devcontainer.compose must be a file path")
	}
	if len(c.LegacyInline) > 0 {
		return (Spec{Compose: c.LegacyInline}).Validate()
	}
	return nil
}

func (c Config) Path(root string) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	path := c.Compose
	if path == "" {
		path = Filename
	}
	if strings.HasPrefix(path, "~/") || strings.Contains(path, "${HOME}") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = strings.ReplaceAll(path, "${HOME}", home)
		if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Abs(path)
}

func (c Config) Snapshot(root string) (Spec, error) {
	if len(c.LegacyInline) > 0 {
		return Spec{}, fmt.Errorf("inline devcontainer.compose is no longer supported; run cxz edit docker-compose to move it into %s", Filename)
	}
	path, err := c.Path(root)
	if err != nil {
		return Spec{}, err
	}
	b, err := ReadFile(path)
	if os.IsNotExist(err) && c.Compose == "" {
		return Spec{}, nil
	}
	if err != nil {
		return Spec{}, fmt.Errorf("devcontainer.compose: %w", err)
	}
	return SnapshotFile(path, b)
}

func ReadFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return nil, fmt.Errorf("Compose file must be a regular file up to 256 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err == nil && len(b) > MaxBytes {
		err = fmt.Errorf("Compose file exceeds 256 KiB")
	}
	return b, err
}

// SnapshotFile also validates editor drafts using the selected file's directory,
// so include paths and relative binds keep their meaning before and after save.
func SnapshotFile(path string, b []byte) (Spec, error) {
	if len(b) > MaxBytes {
		return Spec{}, fmt.Errorf("Compose file exceeds 256 KiB")
	}
	var original map[string]any
	d := yaml.NewDecoder(bytes.NewReader(b), yaml.Strict())
	if err := d.Decode(&original); err == io.EOF {
		return Spec{}, nil
	} else if err != nil {
		return Spec{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Spec{}, fmt.Errorf("Compose file must contain one YAML/JSON document")
	}
	if len(original) == 0 {
		return Spec{}, nil
	}
	env := types.NewMapping(os.Environ())
	dir := filepath.Dir(path)
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		values, err := dotenv.GetEnvFromFile(env, []string{filepath.Join(dir, ".env")})
		if err != nil {
			return Spec{}, err
		}
		env = env.Merge(values)
	}
	model, err := loader.LoadModelWithContext(context.Background(), types.ConfigDetails{
		WorkingDir: dir, Environment: env,
		ConfigFiles: []types.ConfigFile{{Filename: path, Content: b}},
	}, func(o *loader.Options) {
		// A fragment need not define an image. The manager validates the combined
		// project model after substituting the per-project service name.
		o.SkipValidation = true
		o.SkipNormalization = true
		o.SkipConsistencyCheck = true
		o.SkipDefaultValues = true
		o.ResolvePaths = true
		o.SetProjectName("cxz-override", true)
	})
	if err != nil {
		return Spec{}, fmt.Errorf("Compose override: %w", err)
	}
	delete(model, "name") // cxz owns project names.
	if services, ok := model["services"].(map[string]any); ok && len(services) == 0 {
		delete(model, "services")
	}
	if len(model) == 0 {
		return Spec{}, nil
	}
	// Compose has already interpolated on the host. Escape literal dollars so
	// the manager's later project merge cannot interpolate them a second time.
	var escape func(any) any
	escape = func(value any) any {
		switch x := value.(type) {
		case string:
			return strings.ReplaceAll(x, "$", "$$")
		case []any:
			for i := range x {
				x[i] = escape(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = escape(x[k])
			}
		}
		return value
	}
	raw, err := json.Marshal(escape(model))
	if err != nil {
		return Spec{}, err
	}
	s := Spec{Compose: raw}
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
