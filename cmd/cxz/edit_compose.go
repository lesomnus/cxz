package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/settings"
)

func editDockerCompose(root string, open func(string) error) (string, bool, error) {
	settingsBefore, settingsSource, err := settings.Read(root)
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	settingsExisted := err == nil
	var cfg settings.Config
	if settingsExisted {
		cfg, err = settings.Parse(settingsBefore)
		if err != nil {
			return "", false, err
		}
	}
	path, err := cfg.Devcontainer.Path(root)
	if err != nil {
		return "", false, err
	}
	selectedPath := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return path, false, err
	}
	original, err := projectconfig.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, false, err
	}
	existed := err == nil
	initial := original
	if !existed {
		initial = projectconfig.Template()
	}
	migrate := len(cfg.Devcontainer.LegacyInline) > 0
	if migrate {
		if existed {
			return path, false, fmt.Errorf("legacy inline override was preserved: %s already exists; move the inline settings into that file using cxz edit", path)
		}
		var value map[string]any
		if err = json.Unmarshal(cfg.Devcontainer.LegacyInline, &value); err != nil {
			return path, false, err
		}
		initial, err = yaml.Marshal(value)
		if err != nil {
			return path, false, err
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cxz-compose-edit-*.yaml")
	if err != nil {
		return path, false, err
	}
	draft := f.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(draft)
		}
	}()
	if _, err = f.Write(initial); err != nil {
		f.Close()
		return path, false, err
	}
	if err = f.Close(); err != nil {
		return path, false, err
	}
	fail := func(err error) (string, bool, error) {
		keep = true
		return path, false, fmt.Errorf("Compose file unchanged; draft kept at %s: %w", draft, err)
	}
	if err = open(draft); err != nil {
		return fail(err)
	}
	edited, err := projectconfig.ReadFile(draft)
	if err != nil {
		return fail(err)
	}
	if _, err = projectconfig.SnapshotFile(selectedPath, edited); err != nil {
		return fail(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return fail(err)
	}
	lock, err := core.Lock(filepath.Join(root, "settings.lock"))
	if err != nil {
		return fail(err)
	}
	defer lock.Close()
	fileLock, err := core.Lock(path + ".cxz.lock")
	if err != nil {
		return fail(err)
	}
	defer fileLock.Close()
	currentSettings, currentSource, err := settings.Read(root)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	if currentSource != settingsSource || (err == nil) != settingsExisted || !bytes.Equal(currentSettings, settingsBefore) {
		return fail(fmt.Errorf("settings changed while editing; reopen cxz edit docker-compose"))
	}
	current, err := projectconfig.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	if (err == nil) != existed || !bytes.Equal(current, original) {
		return fail(fmt.Errorf("Compose file changed while editing; reopen cxz edit docker-compose"))
	}
	changed := !existed || !bytes.Equal(edited, original)
	if changed {
		if err = core.WriteFile(path, edited); err != nil {
			return fail(err)
		}
	}
	if migrate {
		cfg.Devcontainer = projectconfig.Config{Compose: projectconfig.Filename}
		if err = settings.Save(root, cfg); err != nil {
			return path, true, fmt.Errorf("Compose saved to %s; update devcontainer.compose to that path with cxz edit: %w", path, err)
		}
	}
	return path, changed, nil
}
