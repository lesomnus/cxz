package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type loginSignalWriter struct{ ready chan struct{} }

func (w loginSignalWriter) Write(p []byte) (int, error) {
	select {
	case w.ready <- struct{}{}:
	default:
	}
	return len(p), nil
}

func TestRuntimeSessionLoginPublishesOnlyCompletedProfile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB, create_id TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	state := filepath.Join(root, "data")
	provider := filepath.Join(root, "claude")
	// Synthetic provider: exercise the real login subprocess, isolated environment
	// and atomic credential install without contacting a provider or using a token.
	script := `#!/bin/sh
set -eu
printf 'Visit https://example.invalid/login\n'
IFS= read -r code
test "$code" = 'fixture#state'
printf '%s' '{"claudeAiOauth":{"accessToken":"synthetic"}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
`
	if err := os.WriteFile(provider, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(root, "runtime.json"), workspace.Runtime{ProjectID: "project", Workspace: root, Claude: provider}); err != nil {
		t.Fatal(err)
	}
	s := &Server{root: state, db: db}
	in, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	defer w.Close()
	if _, err := io.WriteString(w, "fixture#state\n"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := s.LoginSession(ctx, "project", "work1", "creation", in, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "https://example.invalid/login") || strings.Contains(output.String(), "fixture#state") || strings.Contains(output.String(), "synthetic") {
		t.Fatal("URL missing or credentials exposed", output.String())
	}
	if _, err := accounts.Credential(accounts.SessionRoot(state, "creation"), "work1", "claude"); err != nil {
		t.Fatal("credentials not published to requested session", err)
	}
	if _, err := accounts.Credential(state, "work1", "claude"); err == nil {
		t.Fatal("login populated the project-wide profile")
	}
	if err := s.LoginSession(ctx, "other-project", "work1", "creation", in, io.Discard); status.Code(err) != codes.PermissionDenied {
		t.Fatal("foreign project accepted", err)
	}
	lease, err := core.Lock(filepath.Join(accounts.Dir(accounts.SessionRoot(state, "creation"), "work1"), "login.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LoginSession(ctx, "project", "work1", "creation", in, io.Discard); err == nil {
		t.Fatal("active profile lock bypassed")
	}
	lease.Close()
	raw, _ := json.Marshal(core.Session{ID: "session", ProjectID: "project", Kind: "claude", Account: "other", AuthBackend: accounts.ProjectLocalOAuth})
	if _, err := db.Exec("INSERT INTO sessions VALUES(?,?,?)", "session", raw, "existing"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoginSession(ctx, "project", "work1", "existing", in, io.Discard); status.Code(err) != codes.PermissionDenied {
		t.Fatal("existing session account changed", err)
	}
	request, abort := context.WithCancel(ctx)
	defer abort()
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- s.LoginSession(request, "project", "work1", "cancelled", in, loginSignalWriter{ready}) }()
	select {
	case <-ready:
		abort()
	case <-ctx.Done():
		t.Fatal("provider did not start")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled login succeeded")
		}
	case <-ctx.Done():
		t.Fatal("cancelled login process did not exit")
	}
	if _, err := accounts.Credential(accounts.SessionRoot(state, "cancelled"), "work1", "claude"); err == nil {
		t.Fatal("cancelled login published credentials")
	}
}
