package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
)

func TestWindowsEditCommandWithBatchEditorAndLiteralPath(t *testing.T) {
	for _, variable := range []string{"VISUAL", "EDITOR"} {
		t.Run(variable, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state 한글 with spaces & ^ %CXZ_CANARY% !CXZ_CANARY! (draft)")
			script := filepath.Join(t.TempDir(), "editor with spaces.cmd")
			body := "@echo off\r\nif not \"%~1\"==\"--wait\" exit /b 2\r\nif \"%~2\"==\"\" exit /b 3\r\nif not \"%~3\"==\"\" exit /b 4\r\n> \"%~2\" echo {\"claude_model\":\"editor-model\",\"connections\":{\"work\":{\"target\":\"ssh://work\"}}}\r\n"
			if err := os.WriteFile(script, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CXZ_CANARY", "must-not-expand")
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", "exit /b 42")
			t.Setenv(variable, `"`+script+`" --wait`)
			got := xlitest.Run(t, newRoot(root), "edit")
			if got.Err != nil {
				t.Fatal(got)
			}
			cfg, err := settings.Load(root)
			if err != nil || cfg.ClaudeModel != "editor-model" || cfg.Connections.DefaultName() != "work" {
				t.Fatal(cfg, err)
			}
			// Replacing an existing settings file is supported too.
			if err := os.WriteFile(script, []byte(strings.ReplaceAll(body, "editor-model", "second-model")), 0600); err != nil {
				t.Fatal(err)
			}
			if got := xlitest.Run(t, newRoot(root), "edit"); got.Err != nil {
				t.Fatal(got)
			}
		})
	}
}
func TestWindowsEditHelpAndEditorFailure(t *testing.T) {
	root := t.TempDir()
	help := xlitest.Run(t, newRoot(root), "edit", "--help")
	if help.Err != nil || !strings.Contains(help.Stdout, "Notepad") {
		t.Fatal(help)
	}
	t.Setenv("VISUAL", "exit /b 17")
	result := xlitest.Run(t, newRoot(root), "edit")
	if result.Err == nil || !strings.Contains(result.Err.Error(), "draft kept") {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(root, "settings.jsonc")); !os.IsNotExist(err) {
		t.Fatal("failed editor changed settings", err)
	}
}
func TestNotepadCompletionAndCancellation(t *testing.T) {
	c := &xli.Command{ReadCloser: io.NopCloser(strings.NewReader("\r\n"))}
	if err := finishNotepadEdit(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	c.ReadCloser = io.NopCloser(strings.NewReader(""))
	if err := finishNotepadEdit(context.Background(), c); err == nil {
		t.Fatal("EOF accepted as completion")
	}
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.ReadCloser = r
	if err := finishNotepadEdit(ctx, c); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestWindowsEditRetainsHostSettingsWithoutOpeningHostPaths(t *testing.T) {
	root := t.TempDir()
	const cfg = `{"connections":{"work":{"target":"ssh://work"}},"files":[{"src":"/host-only/CLAUDE.md","dst":"${AGENT_CONFIG_DIR}/CLAUDE.md"}],"docker":{"compose":"/host-only/compose.yaml"}}`
	changed, bundle, err := editSettings(root, func(p string) error { return os.WriteFile(p, []byte(cfg), 0600) })
	if err != nil || !changed || bundle != nil {
		t.Fatal(changed, bundle, err)
	}
}

func TestWindowsEditShareCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state 한글 with spaces & %CXZ_CANARY%")
	script := filepath.Join(t.TempDir(), "share editor.cmd")
	if err := os.WriteFile(script, []byte("@echo off\r\n> \"%~1\" echo shared instructions\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CXZ_CANARY", "must-not-expand")
	t.Setenv("VISUAL", `"`+script+`"`)
	got := xlitest.Run(t, newRoot(root), "edit", "share", "foo/bar/baz.txt")
	if got.Err != nil {
		t.Fatal(got)
	}
	b, err := os.ReadFile(filepath.Join(root, "share", "foo", "bar", "baz.txt"))
	if err != nil || strings.TrimSpace(string(b)) != "shared instructions" {
		t.Fatal(string(b), err)
	}
}
