package accounts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestIndependentSessionProfiles(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			s := core.Session{CreateID: "one", Kind: agent, Account: "work", ProjectID: "project", AuthBackend: ProjectLocalOAuth, AuthBinding: BindingID("project", "work", ProjectLocalOAuth)}
			credential := []byte(`{"claudeAiOauth":{"accessToken":"synthetic-one"}}`)
			if agent == "codex" {
				credential = []byte(`{"tokens":{"access_token":"synthetic-one"}}`)
			}
			if err := Install(root, s.Account, agent, credential); err != nil {
				t.Fatal(err)
			}
			if _, err := LaunchSession(root, s, nil); err == nil {
				t.Fatal("inherited project credential")
			}
			if err := Install(AuthRoot(root, s), s.Account, agent, credential); err != nil {
				t.Fatal(err)
			}
			first, err := LaunchSession(root, s, []string{"CLAUDE_SECURESTORAGE_CONFIG_DIR=/wrong", "TMPDIR=/shared", "CODEX_HOME=/wrong"})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range first.Env {
				if strings.Contains(e, "/wrong") || strings.Contains(e, "/shared") {
					t.Fatal("inherited state", e)
				}
			}
			s.CreateID = "two"
			if _, err := LaunchSession(root, s, nil); err == nil {
				t.Fatal("inherited first session login")
			}
			if err := Install(AuthRoot(root, s), s.Account, agent, credential); err != nil {
				t.Fatal(err)
			}
			second, err := LaunchSession(root, s, nil)
			if err != nil || first.ConfigDir == second.ConfigDir {
				t.Fatal("shared session configuration", err)
			}
			s.CreateID = "one"
			again, err := LaunchSession(root, s, nil)
			if err != nil || again.ConfigDir != first.ConfigDir {
				t.Fatal("resume changed profile", err)
			}
			lease, err := core.Lock(filepath.Join(Dir(AuthRoot(root, s), s.Account), "login.lock"))
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			b, _ := Resolve(agent, ProjectLocalOAuth)
			if err := b.Login(context.Background(), LoginRequest{Root: AuthRoot(root, s), Account: s.Account, Workspace: "/work", Binary: "/bin/true"}); err == nil {
				t.Fatal("relogin while session active")
			}
		})
	}
}

func TestBrokeredSessionHomes(t *testing.T) {
	root := t.TempDir()
	g := Grant{Socket: BrokerSocket, Capability: strings.Repeat("a", 64), Project: "project", Account: "work", Binding: BindingID("project", "work", BrokeredAccessToken), AccountID: "synthetic"}
	if err := InstallGrant(root, g); err != nil {
		t.Fatal(err)
	}
	s := core.Session{CreateID: "one", Kind: "codex", Account: g.Account, ProjectID: g.Project, AuthBackend: BrokeredAccessToken, AuthBinding: g.Binding}
	a, err := LaunchSession(root, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.CreateID = "two"
	b, err := LaunchSession(root, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ConfigDir == b.ConfigDir {
		t.Fatal("shared Codex home")
	}
	for _, p := range []string{a.ConfigDir, b.ConfigDir} {
		if _, err := os.Stat(filepath.Join(p, "auth.json")); !os.IsNotExist(err) {
			t.Fatal("persisted broker token")
		}
	}
}
