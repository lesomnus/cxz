package sessionaccount_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionaccount"
)

func fixture(t *testing.T, agent string) (string, core.Session, string) {
	t.Helper()
	root := t.TempDir()
	session := core.Session{ID: strings.Repeat("a", 24), CreateID: "create", Kind: agent, Account: "work", ProjectID: "project", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)}
	credential := []byte(`{"claudeAiOauth":{"accessToken":"synthetic"}}`)
	if agent == "codex" {
		credential = []byte(`{"tokens":{"access_token":"synthetic"}}`)
	}
	profile := accounts.SessionRoot(root, session.CreateID)
	if err := accounts.Install(profile, session.Account, agent, credential); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(accounts.Config(profile, session.Account), sessionaccount.HistoryFolder(agent), "workspace", "native.jsonl")
	if err := os.MkdirAll(filepath.Dir(native), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("conversation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root, session, native
}

func target(account string) sessionaccount.Target {
	return sessionaccount.Target{Account: account, AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID("project", account, accounts.ProjectLocalOAuth)}
}

func TestSwitchMovesHistoryAndLeavesCredentials(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root, session, native := fixture(t, agent)
			out, err := sessionaccount.Switch(root, session, target("personal"))
			if err != nil {
				t.Fatal(err)
			}
			if out.Account != "personal" || out.AuthBinding != target("personal").AuthBinding {
				t.Fatalf("manifest not rebound: %+v", out)
			}
			profile := accounts.SessionRoot(root, session.CreateID)
			moved := filepath.Join(accounts.Config(profile, "personal"), sessionaccount.HistoryFolder(agent), "workspace", "native.jsonl")
			if b, err := os.ReadFile(moved); err != nil || string(b) != "conversation\n" {
				t.Fatalf("history did not move: %q %v", b, err)
			}
			if _, err := os.Stat(native); !os.IsNotExist(err) {
				t.Fatalf("old history still present: %v", err)
			}
			entries, err := os.ReadDir(accounts.Config(profile, "personal"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != sessionaccount.HistoryFolder(agent) {
					t.Fatalf("unexpected file carried to the new account: %s", entry.Name())
				}
			}
			// Retrying after the history moved, before the manifest was saved.
			again, err := sessionaccount.Switch(root, session, target("personal"))
			if err != nil || again.Account != "personal" {
				t.Fatalf("retry: %+v %v", again, err)
			}
			// And back again.
			back, err := sessionaccount.Switch(root, out, target("work"))
			if err != nil || back.Account != "work" {
				t.Fatalf("switch back: %+v %v", back, err)
			}
			if _, err := os.Stat(native); err != nil {
				t.Fatalf("history did not return: %v", err)
			}
		})
	}
}

func TestSwitchRefusesToMergeHistory(t *testing.T) {
	root, session, _ := fixture(t, "claude")
	profile := accounts.SessionRoot(root, session.CreateID)
	other := filepath.Join(accounts.Config(profile, "personal"), "projects", "x.jsonl")
	if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionaccount.Switch(root, session, target("personal")); err == nil {
		t.Fatal("merged into existing history")
	}
}

func TestSwitchValidatesBinding(t *testing.T) {
	root, session, _ := fixture(t, "claude")
	bad := target("personal")
	bad.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	if _, err := sessionaccount.Switch(root, session, bad); err == nil {
		t.Fatal("accepted a binding for another account")
	}
}

func TestSwitchRewritesUnstartedForkPath(t *testing.T) {
	root, session, native := fixture(t, "codex")
	session.Fork = &core.ForkOrigin{SessionID: "parent", VendorID: "v", NativePath: native}
	out, err := sessionaccount.Switch(root, session, target("personal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out.Fork.NativePath); err != nil {
		t.Fatalf("fork path not rewritten: %s %v", out.Fork.NativePath, err)
	}
	if session.Fork.NativePath != native {
		t.Fatal("input manifest mutated")
	}
}
