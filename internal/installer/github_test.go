package installer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
)

func TestGitHubSyncUsesStdinNotArguments(t *testing.T) {
	ghFixture(t, "github.com:\n  oauth_token: secret-fixture\n", "")
	bin := os.Getenv("PATH")
	target := t.TempDir()
	t.Setenv("CXZ_TEST_STDIN", filepath.Join(target, "stdin"))
	t.Setenv("CXZ_TEST_ARGS", filepath.Join(target, "args"))
	script := `#!/bin/sh
case "$1" in
inspect) printf '%s' '[{"Config":{"Labels":{"cxz.owner":"test-owner","cxz.role":"daemon"}}}]' ;;
exec) /bin/cat > "$CXZ_TEST_STDIN"; printf '%s\n' "$@" > "$CXZ_TEST_ARGS" ;;
*) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "installation.json"), transport.Installation{Owner: "test-owner", Container: "test-manager"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := SyncGitHub(context.Background(), root, &out); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(os.Getenv("CXZ_TEST_ARGS"))
	in, _ := os.ReadFile(os.Getenv("CXZ_TEST_STDIN"))
	if strings.Contains(string(args)+out.String(), "secret-fixture") || !strings.Contains(string(in), "secret-fixture") {
		t.Fatal("credential transport not private")
	}
}

func ghFixture(t *testing.T, config, script string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST"} {
		t.Setenv(key, "")
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if script != "" {
		if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGitHubSnapshotKeychainAndSanitization(t *testing.T) {
	ghFixture(t, "github.com:\n  user: work\n  users:\n    inactive:\n      oauth_token: do-not-export\n", `printf '%s\n' keychain-fixture`)
	b, err := githubSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "keychain-fixture") || strings.Contains(string(b), "do-not-export") {
		t.Fatal("wrong active credential export")
	}
	ghFixture(t, "github.com:\n  user: work\n", `echo super-secret-token >&2; exit 1`)
	_, err = githubSnapshot(context.Background())
	if err == nil || strings.Contains(err.Error(), "super-secret") {
		t.Fatal("missing or unsafe error")
	}
}

func TestGitHubSnapshotEnvAndLogout(t *testing.T) {
	ghFixture(t, "github.com:\n  oauth_token: old-fixture\n", "")
	t.Setenv("GH_TOKEN", "environment-fixture")
	b, err := githubSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "environment-fixture") || strings.Contains(string(b), "old-fixture") {
		t.Fatal("environment precedence lost")
	}
	ghFixture(t, "{}", "")
	b, err = githubSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var hosts map[string]any
	if yaml.Unmarshal(b, &hosts) != nil || len(hosts) != 0 {
		t.Fatal("logout did not produce empty snapshot")
	}
}

func TestGitHubSnapshotPlainFileWithoutCLI(t *testing.T) {
	ghFixture(t, "github.com:\n  oauth_token: file-fixture\n  user: work\n", "")
	b, err := githubSnapshot(context.Background())
	if err != nil || !strings.Contains(string(b), "file-fixture") {
		t.Fatal("file export failed")
	}
}
