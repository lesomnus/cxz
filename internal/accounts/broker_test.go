package accounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func seedCentral(t *testing.T, root, account, subject, access string) {
	t.Helper()
	claims, _ := json.Marshal(map[string]string{"sub": "user-" + account})
	raw, _ := json.Marshal(map[string]any{"tokens": map[string]string{"id_token": "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".synthetic", "access_token": access, "refresh_token": "synthetic-refresh-secret", "account_id": subject}})
	if err := Install(centralRoot(root), account, "codex", raw); err != nil {
		t.Fatal(err)
	}
	pin, err := credentialSubject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = core.WriteJSON(filepath.Join(Dir(centralRoot(root), account), "subject.json"), pin); err != nil {
		t.Fatal(err)
	}
}
func TestBrokerIsolationRefreshAndRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	projectRoot := t.TempDir()
	socket := filepath.Join(t.TempDir(), "broker.sock")
	seedCentral(t, root, "work", "subject-work", "synthetic-old")
	seedCentral(t, root, "personal", "subject-personal", "synthetic-personal")
	grant, err := IssueGrant(root, "project-a", "work")
	if err != nil {
		t.Fatal(err)
	}
	same, err := IssueGrant(root, "project-a", "work")
	if err != nil || same != grant {
		t.Fatal("grant retry", err)
	}
	other, err := IssueGrant(root, "project-b", "work")
	if err != nil || other.Capability == grant.Capability || other.Binding == grant.Binding {
		t.Fatal("project isolation", err)
	}
	if err = InstallGrant(projectRoot, grant); err != nil {
		t.Fatal(err)
	}
	grant.Socket = socket
	if err = core.WriteJSON(grantPath(projectRoot, "work"), grant); err != nil {
		t.Fatal(err)
	}
	var calls, active, maxActive atomic.Int32
	refresh := func(ctx context.Context, account string) error {
		n := active.Add(1)
		defer active.Add(-1)
		if n > maxActive.Load() {
			maxActive.Store(n)
		}
		time.Sleep(20 * time.Millisecond)
		calls.Add(1)
		seedCentral(t, root, account, "subject-work", "synthetic-fresh")
		return nil
	}
	server, err := StartBroker(root, socket, refresh)
	if err != nil {
		t.Fatal(err)
	}
	fetch := func(previous string, refresh bool) (Token, error) {
		return FetchToken(ctx, projectRoot, "work", "project-a", grant.Binding, previous, refresh)
	}
	tok, err := fetch("", false)
	if err != nil || tok.AccessToken != "synthetic-old" {
		t.Fatal("initial token", err)
	}
	if _, err = fetch("subject-personal", true); err == nil {
		t.Fatal("cross-account refresh accepted")
	}
	if _, err = FetchToken(ctx, projectRoot, "work", "project-b", grant.Binding, "", false); err == nil {
		t.Fatal("cross-project accepted")
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fetch("subject-work", true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 3 || maxActive.Load() != 1 {
		t.Fatal("refresh not serialized")
	}
	if err = server.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = fetch("", false); err == nil {
		t.Fatal("manager outage silently accepted")
	}
	server, err = StartBroker(root, socket, refresh)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	tok, err = fetch("", false)
	if err != nil || tok.AccessToken != "synthetic-fresh" {
		t.Fatal("restart", err)
	}
	if _, err = os.Stat(filepath.Join(Config(projectRoot, "work"), "auth.json")); !os.IsNotExist(err) {
		t.Fatal("credentials copied to project")
	}
	raw, _ := os.ReadFile(grantPath(projectRoot, "work"))
	if strings.Contains(string(raw), "synthetic-refresh") || strings.Contains(string(raw), "synthetic-fresh") {
		t.Fatal("token persisted in grant")
	}
	grant.Capability = strings.Repeat("0", 64)
	_ = core.WriteJSON(grantPath(projectRoot, "work"), grant)
	if _, err = fetch("", false); err == nil {
		t.Fatal("forged capability accepted")
	}
}

func TestCentralLoginRejectsDifferentSubject(t *testing.T) {
	root := t.TempDir()
	if err := core.Prepare(root); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "codex-fixture")
	script := `#!/bin/sh
printf '%s' '{"tokens":{"id_token":"e30.eyJzdWIiOiJ1c2VyLW90aGVyIn0.synthetic","access_token":"synthetic-new","refresh_token":"synthetic-refresh","account_id":"subject-work"}}' > "$CODEX_HOME/auth.json"
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	seedCentral(t, root, "work", "subject-work", "synthetic-original")
	err := CentralLogin(context.Background(), LoginRequest{Root: root, Account: "work", Binary: binary, Env: os.Environ()})
	if err == nil || !strings.Contains(err.Error(), "different ChatGPT account") {
		t.Fatal("different subject login not rejected by identity check", err)
	}
	tok, err := CentralToken(root, "work")
	if err != nil || tok.AccessToken != "synthetic-original" {
		t.Fatal("existing login overwritten", err)
	}
}
