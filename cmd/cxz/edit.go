package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/settings"
)

// Edit a private working copy, preserving the original on invalid edits or
// concurrent changes. Keep failed drafts so the user can recover their work.
func editSettings(root string, open func(string) error) (bool, *filemap.Bundle, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return false, nil, err
	}
	path := filepath.Join(root, "settings.json")
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, nil, err
	}
	initial := original
	if len(initial) == 0 {
		initial = settings.Template()
	}
	tmp, err := os.CreateTemp(root, ".cxz-edit-*.json")
	if err != nil {
		return false, nil, err
	}
	draft := tmp.Name()
	if _, err = tmp.Write(initial); err != nil {
		tmp.Close()
		os.Remove(draft)
		return false, nil, err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(draft)
		return false, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(draft)
		}
	}()
	fail := func(err error) (bool, *filemap.Bundle, error) {
		keep = true
		return false, nil, fmt.Errorf("settings unchanged; draft kept at %s: %w", draft, err)
	}
	if err = open(draft); err != nil {
		return fail(err)
	}
	edited, err := os.ReadFile(draft)
	if err != nil {
		return fail(err)
	}
	if bytes.Equal(edited, original) {
		return false, nil, nil
	}
	cfg, err := settings.Parse(edited)
	if err != nil {
		return fail(err)
	}
	bundle, err := prepareSettingsEdit(root, original, cfg)
	if err != nil {
		return fail(err)
	}
	lock, err := core.Lock(filepath.Join(root, "settings.lock"))
	if err != nil {
		return fail(err)
	}
	defer lock.Close()
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	if !bytes.Equal(current, original) {
		return fail(fmt.Errorf("settings changed while the editor was open; reopen cxz edit"))
	}
	if err = os.Chmod(draft, 0600); err != nil {
		return fail(err)
	}
	f, err := os.OpenFile(draft, os.O_RDWR, 0)
	if err != nil {
		return fail(err)
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	if err = os.Rename(draft, path); err != nil {
		return fail(err)
	}
	if err = core.SyncDir(root); err != nil {
		return true, bundle, fmt.Errorf("settings saved but directory sync failed: %w", err)
	}
	return true, bundle, nil
}
