package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func editCommand() *xli.Command {
	return &xli.Command{Name: "edit", Brief: "Edit local settings.jsonc using VISUAL/EDITOR or Notepad", Commands: xli.Commands{{Name: "docker-compose", Brief: "Edit the local project Compose override file", Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		root := c
		for root.HasParent() {
			root = root.Parent()
		}
		state, err := filepath.Abs(flg.MustGet[string](root, "state"))
		if err != nil {
			return err
		}
		path, changed, err := editDockerCompose(state, func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintln(c.Writer, "Saved", path)
		} else {
			fmt.Fprintln(c.Writer, "No changes:", path)
		}
		fmt.Fprintln(c.Writer, "Saved locally; publish host settings using cxz on the Linux daemon host.")
		return nil
	})}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		root := c
		for root.HasParent() {
			root = root.Parent()
		}
		state, err := filepath.Abs(flg.MustGet[string](root, "state"))
		if err != nil {
			return err
		}
		changed, _, err := editSettings(state, func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintln(c.Writer, "Saved", settings.Path(state))
		} else {
			fmt.Fprintln(c.Writer, "No changes.")
		}
		return nil
	})}
}

// Host file mappings and Docker overrides are applied by the Linux host CLI.
// Editing frontend settings never reads host paths or connects to a daemon.
func prepareSettingsEdit(_ string, _ []byte, _ settings.Config) (*filemap.Bundle, error) {
	return nil, nil
}

func openSettingsEditor(ctx context.Context, c *xli.Command, path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	var cmd *exec.Cmd
	notepad := editor == ""
	if notepad {
		cmd = exec.CommandContext(ctx, "notepad.exe", path)
		fmt.Fprintln(c.Writer, "Save your changes and close Notepad, then press Enter here to finish.")
	} else {
		cmd = windowsEditorCommand(ctx, editor, path)
	}
	if !notepad {
		cmd.Stdin = c.ReadCloser
	}
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter
	if err := cmd.Run(); err != nil {
		return err
	}
	if notepad {
		// Recent Notepad versions may hand the file to an existing app instance and
		// exit before editing finishes. Keep the draft until the user is finished.
		return finishNotepadEdit(ctx, c)
	}
	return nil
}

func windowsEditorCommand(ctx context.Context, editor, path string) *exec.Cmd {
	shell := os.Getenv("ComSpec")
	if shell == "" {
		shell = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, shell)
	// cmd.exe uses different quoting from CommandLineToArgvW. Pass a literal
	// command line, disable AutoRun/delayed expansion, and expand the filename
	// once from the environment so %, !, &, spaces and Unicode stay data.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(cmd.Path) + ` /d /v:off /s /c "` + editor + ` "%CXZ_EDITOR_FILE%""`}
	cmd.Env = append(os.Environ(), "CXZ_EDITOR_FILE="+path)
	return cmd
}
func finishNotepadEdit(ctx context.Context, c *xli.Command) error {
	done := make(chan error, 1)
	go func() { _, err := bufio.NewReader(c.ReadCloser).ReadString('\n'); done <- err }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("could not confirm editing is finished: %w", err)
		}
		return nil
	}
}
