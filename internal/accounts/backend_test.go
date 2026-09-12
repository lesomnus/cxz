package accounts

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestBackendMappingsAndBindings(t *testing.T) {
	if len(Catalog()) != 2 {
		t.Fatal("agent catalog")
	}
	for _, agent := range []string{"claude", "codex"} {
		b, err := Select(agent, "")
		if err != nil {
			t.Fatal(err)
		}
		if b.Info().ID != ProjectLocalOAuth || b.Info().RefreshOwner != "agent" || b.Info().Workflow != "project-login" {
			t.Fatal("wrong strategy")
		}
		for _, unsupported := range []string{"", BrokeredAccessToken, APIKey, "typo"} {
			if _, err := Resolve(agent, unsupported); err == nil {
				t.Fatal("silent backend fallback", unsupported)
			}
		}
		a, _ := b.Binding("project-a", "personal")
		same, _ := b.Binding("project-a", "personal")
		other, _ := b.Binding("project-b", "personal")
		work, _ := b.Binding("project-a", "work")
		if a != same || a.ID == other.ID || a.ID == work.ID || a.Scope != "project" || a.CredentialRef != "accounts/personal" {
			t.Fatal("binding scope/identity")
		}
		if _, err := b.Binding("", "personal"); err == nil {
			t.Fatal("project-less OAuth")
		}
		if _, err := ResolveBinding(agent, ProjectLocalOAuth, "project-b", "personal", a.ID); err == nil {
			t.Fatal("cross-project binding")
		}
	}
	if _, err := Select("unknown", ""); err == nil {
		t.Fatal("unknown agent accepted")
	}
}
func TestBackendLoginAndLaunch(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			if err := core.Prepare(root); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(root, "login-fixture")
			script := `#!/bin/sh
set -eu
case "$*" in
 'auth login') printf '%s' '{"claudeAiOauth":{"accessToken":"synthetic-login"}}' > "$CLAUDE_CONFIG_DIR/.credentials.json" ;;
 '-c cli_auth_credentials_store="file" login --device-auth') printf '%s' '{"tokens":{"access_token":"synthetic-login"}}' > "$CODEX_HOME/auth.json" ;;
 *) exit 90 ;;
esac
test -z "${OPENAI_API_KEY:-}"
test -z "${ANTHROPIC_API_KEY:-}"
`
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			b, _ := Resolve(agent, ProjectLocalOAuth)
			if err := b.Check(root, "work"); err == nil {
				t.Fatal("missing credentials accepted")
			}
			req := LoginRequest{Root: root, Account: "work", Workspace: "/workspace", Binary: binary, Env: []string{"PATH=/usr/bin:/bin", "OPENAI_API_KEY=wrong-account", "ANTHROPIC_API_KEY=wrong-account"}}
			if err := b.Login(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			if err := b.Check(root, "work"); err != nil {
				t.Fatal(err)
			}
			launch, err := b.Launch(root, "work", req.Env)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Join(launch.Env, "\n"), "wrong-account") || launch.ConfigDir != Config(root, "work") {
				t.Fatal("incorrect auth environment")
			}
			if agent == "codex" && !reflect.DeepEqual(launch.Args, []string{"-c", `cli_auth_credentials_store="file"`, "-c", `model_provider="openai"`}) {
				t.Fatal("Codex auth overrides", launch.Args)
			}
			before, err := Credential(root, "work", agent)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(binary, []byte(script+"exit 17\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err = b.Login(context.Background(), req); err == nil {
				t.Fatal("failed login accepted")
			}
			after, _ := Credential(root, "work", agent)
			if string(before) != string(after) {
				t.Fatal("failed login changed credentials")
			}
			leftovers, _ := filepath.Glob(filepath.Join(Dir(root, "work"), "login-*"))
			if len(leftovers) != 0 {
				t.Fatal("staged credentials leaked", leftovers)
			}
		})
	}
}
