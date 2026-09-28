package projectconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

const TemplateDirectory = "devcontainer"
const ProjectNameVariable = "${cxz:projectName}"
const TemplateReceipt = "Default devcontainer template saved"
const MaxTemplateBytes = 1024 * 1024
const maxTemplateFiles = 256

type TemplateFile struct {
	Data       []byte `json:"data"`
	Executable bool   `json:"executable,omitempty"`
}
type DefaultTemplate struct {
	Files map[string]TemplateFile `json:"files"`
}

func snapshotTemplate(root string) (*DefaultTemplate, error) {
	dir := filepath.Join(root, TemplateDirectory)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("devcontainer template must be a directory: %s", dir)
	}
	t := &DefaultTemplate{Files: map[string]TemplateFile{}}
	total := 0
	err = filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("template files must be regular files (no symlinks): %s", file)
		}
		if len(t.Files) >= maxTemplateFiles || info.Size() > int64(MaxTemplateBytes-total) {
			return fmt.Errorf("devcontainer template exceeds 256 files or 1 MiB")
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(f, int64(MaxTemplateBytes-total+1)))
		f.Close()
		if err != nil {
			return err
		}
		total += len(b)
		if total > MaxTemplateBytes {
			return fmt.Errorf("devcontainer template exceeds 1 MiB")
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		t.Files[filepath.ToSlash(rel)] = TemplateFile{Data: b, Executable: info.Mode()&0111 != 0}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, t.Validate()
}

func (t *DefaultTemplate) Validate() error {
	if t == nil {
		return nil
	}
	if len(t.Files) > maxTemplateFiles {
		return fmt.Errorf("devcontainer template exceeds 256 files")
	}
	total := 0
	for name, f := range t.Files {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
			return fmt.Errorf("invalid template path %q", name)
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := t.Files[parent]; exists {
				return fmt.Errorf("template file conflicts with directory %q", parent)
			}
		}
		total += len(f.Data)
		if total > MaxTemplateBytes {
			return fmt.Errorf("devcontainer template exceeds 1 MiB")
		}
	}
	f, exists := t.Files["devcontainer.json"]
	if !exists {
		return fmt.Errorf("devcontainer template requires devcontainer/devcontainer.json")
	}
	cfg, err := templateConfig(f.Data)
	if err != nil {
		return err
	}
	return t.validateReferences(cfg)
}

func templateConfig(b []byte) (map[string]any, error) {
	b, err := hujson.Standardize(bytes.Clone(b))
	if err != nil {
		return nil, fmt.Errorf("default devcontainer.json: %w", err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(b, &cfg); err != nil || cfg == nil {
		return nil, fmt.Errorf("default devcontainer.json must be an object")
	}
	if cfg["image"] == nil && cfg["build"] == nil && cfg["dockerFile"] == nil && cfg["dockerComposeFile"] == nil {
		return nil, fmt.Errorf("default devcontainer.json requires image, build, or dockerComposeFile")
	}
	return cfg, nil
}

// Template-relative configuration files must travel with the snapshot. Native
// devcontainer variables and absolute paths are resolved during provisioning.
func (t *DefaultTemplate) validateReferences(cfg map[string]any) error {
	check := func(value any, directory bool) error {
		name, ok := value.(string)
		if !ok || name == "" {
			return fmt.Errorf("template configuration path must be a non-empty string")
		}
		if strings.Contains(name, "${") || filepath.IsAbs(name) {
			return nil
		}
		name = path.Clean(name)
		if name != "." && !fs.ValidPath(name) {
			return fmt.Errorf("template reference escapes its directory: %s", name)
		}
		if directory {
			if name == "." {
				return nil
			}
			for file := range t.Files {
				if strings.HasPrefix(file, name+"/") {
					return nil
				}
			}
		} else if _, exists := t.Files[name]; exists {
			return nil
		}
		return fmt.Errorf("template reference is missing: %s", name)
	}
	if value, ok := cfg["image"]; ok {
		if text, ok := value.(string); !ok || text == "" {
			return fmt.Errorf("template image must be a non-empty string")
		}
	}
	for _, key := range []string{"name", "remoteUser", "workspaceFolder", "workspaceMount", "service"} {
		if value, exists := cfg[key]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("template %s must be a string", key)
			}
		}
	}
	if value, ok := cfg["dockerFile"]; ok {
		if err := check(value, false); err != nil {
			return err
		}
	}
	if value, ok := cfg["context"]; ok {
		if err := check(value, true); err != nil {
			return err
		}
	}
	if value, ok := cfg["build"]; ok {
		build, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("template build must be an object")
		}
		if err := check(build["dockerfile"], false); err != nil {
			return err
		}
		if value, ok := build["context"]; ok {
			if err := check(value, true); err != nil {
				return err
			}
		}
	}
	if value, ok := cfg["dockerComposeFile"]; ok {
		files, ok := value.([]any)
		if !ok {
			files = []any{value}
		}
		if len(files) == 0 {
			return fmt.Errorf("template dockerComposeFile must not be empty")
		}
		for _, value := range files {
			if err := check(value, false); err != nil {
				return err
			}
		}
		if service, ok := cfg["service"].(string); !ok || service == "" {
			return fmt.Errorf("Compose template requires service")
		}
		if folder, ok := cfg["workspaceFolder"].(string); !ok || folder == "" {
			return fmt.Errorf("Compose template requires workspaceFolder")
		}
	}
	return nil
}

// Render only substitutes parsed JSON strings, so names containing quotes remain
// valid JSON and native devcontainer variables retain their original meaning.
func (t *DefaultTemplate) Render(projectName, workspace string) (map[string]TemplateFile, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	cfg, err := templateConfig(t.Files["devcontainer.json"].Data)
	if err != nil {
		return nil, err
	}
	var replace func(any) any
	replace = func(v any) any {
		switch x := v.(type) {
		case string:
			return strings.ReplaceAll(x, ProjectNameVariable, projectName)
		case []any:
			for i := range x {
				x[i] = replace(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = replace(x[k])
			}
		}
		return v
	}
	replace(cfg)
	// Image/build defaults must bind the actual workspace, never the snapshot.
	if cfg["dockerComposeFile"] == nil {
		folder, _ := cfg["workspaceFolder"].(string)
		if folder == "" {
			folder = "/workspaces/" + filepath.Base(workspace)
			cfg["workspaceFolder"] = folder
		}
		if cfg["workspaceMount"] == nil {
			cfg["workspaceMount"] = "source=" + workspace + ",target=" + folder + ",type=bind"
		}
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	files := make(map[string]TemplateFile, len(t.Files))
	for name, f := range t.Files {
		files[name] = f
	}
	f := files["devcontainer.json"]
	f.Data = b
	files["devcontainer.json"] = f
	return files, nil
}
