package server

import (
	"context"
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

// An account alias is a label; which provider account a project-local login
// reaches is decided by whoever the browser was already signed in to. So a
// login that lands somewhere else has to be refused while its credential is
// still in staging -- after it is installed, the session spends the wrong
// account's limit and says nothing.
func TestSessionLoginRefusesAndPinsProviderAccounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	state := filepath.Join(root, "data")
	provider := filepath.Join(root, "claude")
	script := `#!/bin/sh
set -eu
printf '%s' "{\"claudeAiOauth\":{\"accessToken\":\"synthetic\",\"accountUuid\":\"$CXZ_TEST_ACCOUNT\",\"organizationUuid\":\"org\"}}" > "$CLAUDE_CONFIG_DIR/.credentials.json"
`
	if err := os.WriteFile(provider, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(root, "runtime.json"), workspace.Runtime{ProjectID: "project", Workspace: root, Claude: provider}); err != nil {
		t.Fatal(err)
	}
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB, create_id TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	s := &Server{root: state, db: db}
	mine, _ := accounts.CredentialIdentity("claude", []byte(`{"claudeAiOauth":{"accountUuid":"mine","organizationUuid":"org"}}`))

	login := func(key, reaches, pinned string) (accounts.SessionLoginResult, error) {
		t.Setenv("CXZ_TEST_ACCOUNT", reaches)
		in, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		w.Close()
		return s.LoginSession(ctx, accounts.SessionLogin{Project: "project", Account: "work1", Key: key, Identity: pinned, Input: in, Output: io.Discard})
	}
	credential := func(key string) ([]byte, error) {
		return accounts.Credential(accounts.SessionRoot(state, key), "work1", "claude")
	}

	// Nothing pinned yet: the login is accepted and reports what it reached.
	out, err := login("first", "mine", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Identity != mine {
		t.Fatal("accepted login did not report the account it reached", out.Identity)
	}
	if _, err := credential("first"); err != nil {
		t.Fatal("credential not installed", err)
	}

	// Pinned, and the login reached the same account: accepted, nothing new to
	// write down.
	out, err = login("same", "mine", mine)
	if err != nil {
		t.Fatal(err)
	}
	if out.Identity != "" {
		t.Fatal("unchanged pin reported as a change", out.Identity)
	}

	// Pinned, and the login reached a different one: refused, and the account
	// is left exactly as it was rather than needing to be undone.
	out, err = login("other", "theirs", mine)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal("different provider account accepted", err)
	}
	if !strings.Contains(err.Error(), "clear") {
		t.Fatal("refusal does not say how to change accounts deliberately", err)
	}
	if out.Identity != "" {
		t.Fatal("refused login reported an identity to record", out.Identity)
	}
	if _, err := credential("other"); err == nil {
		t.Fatal("refused login installed its credential")
	}

	// A credential that names no account cannot be checked either way. Refusing
	// it would turn a provider change into logins that stop working, so it is
	// accepted and the pin is left alone.
	out, err = login("silent", "", mine)
	if err != nil {
		t.Fatal("credential without an account should not be refused", err)
	}
	if out.Identity != "" {
		t.Fatal("unknown identity recorded as a pin", out.Identity)
	}
	if _, err := credential("silent"); err != nil {
		t.Fatal("credential not installed", err)
	}
}
