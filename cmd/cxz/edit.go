package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
)

func editCommand() *xli.Command {
	return &xli.Command{Name: "edit", Brief: "Edit settings.json in $VISUAL/$EDITOR; validate and sync changed file mappings", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		root := stateFrom(ctx)
		changed, bundle, err := editSettings(root, func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		if !changed {
			fmt.Fprintln(c.Writer, "No changes.")
			return nil
		}
		fmt.Fprintln(c.Writer, "Saved", filepath.Join(root, "settings.json"))
		if bundle != nil {
			out, err := publishFileMappings(ctx, root, *bundle)
			if err != nil {
				return err
			}
			return writeOutput(c, out)
		}
		return nil
	})}
}

func openSettingsEditor(ctx context.Context, c *xli.Command, path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		for _, name := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(name); err == nil {
				editor = name
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found; set VISUAL or EDITOR")
	}
	// The editor variable is a user-supplied command. Pass the filename separately
	// so spaces, dollar signs, and shell syntax in state paths remain literal.
	cmd := exec.CommandContext(ctx, "sh", "-c", editor+` "$@"`, "cxz-editor", path)
	cmd.Stdin = c.ReadCloser
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter
	return cmd.Run()
}

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
		initial = []byte("{\n  \"files\": []\n}\n")
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
	if bytes.Equal(edited, initial) {
		return false, nil, nil
	}
	cfg, err := settings.Parse(edited)
	if err != nil {
		return fail(err)
	}
	previous, oldErr := settings.Parse(original)
	var bundle *filemap.Bundle
	if (oldErr != nil && len(cfg.Files) > 0) || !slices.Equal(previous.Files, cfg.Files) {
		b, err := filemap.Snapshot(cfg.Files)
		if err != nil {
			return fail(err)
		}
		bundle = &b
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
