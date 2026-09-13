package githubauth

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// This probe uses a disposable owned container and a fake token only. It never
// reads host credentials or makes authenticated GitHub requests.
func TestGitHubDockerInjection(t *testing.T) {
	if os.Getenv("CXZ_GH_DOCKER_TEST") != "1" {
		t.Skip("opt-in Docker/download test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	arch, err := dockerx.Run(ctx, "info", "--format", "{{.Architecture}}")
	if err != nil {
		t.Fatal(err)
	}
	bin, err := distribution.EnsureGitHub(ctx, t.TempDir(), strings.TrimSpace(string(arch)))
	if err != nil {
		t.Fatal(err)
	}
	id := "cxz-gh-test-" + core.ID()[:12]
	_, err = dockerx.Run(ctx, "run", "-d", "--name", id, "--label", "cxz.test="+id, "debian:bookworm-slim", "sleep", "240")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := dockerx.Inspect(context.Background(), id)
		if err == nil && c.Config.Labels["cxz.test"] == id {
			_, err = dockerx.Run(context.Background(), "rm", "-f", id)
			if err != nil {
				t.Error(err)
			}
		}
	})
	f, err := os.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = dockerx.Input(ctx, f, "exec", "-i", id, "sh", "-c", "cat > /usr/local/bin/gh; chmod 755 /usr/local/bin/gh; mkdir -p /cxz/state/data; chown 1000:1000 /cxz/state/data"); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", id, "gh", "--version"); err != nil {
		t.Fatal(err)
	}
	data := []byte("github.com:\n  oauth_token: cxz-fixture-not-a-real-token\n  user: fixture\n  git_protocol: https\n")
	if err = Inject(ctx, id, "1000:1000", data); err != nil {
		t.Fatal(err)
	}
	out, err := dockerx.Run(ctx, "exec", "--user", "1000:1000", "-e", "GH_CONFIG_DIR="+ConfigDir, id, "gh", "auth", "token", "--hostname", "github.com")
	if err != nil || !bytes.Equal(bytes.TrimSpace(out), []byte("cxz-fixture-not-a-real-token")) {
		t.Fatal("native gh did not load injected credential")
	}
	out, err = dockerx.Run(ctx, "exec", id, "stat", "-c", "%a:%u", ConfigDir, ConfigDir+"/hosts.yml")
	if err != nil || string(out) != "700:1000\n600:1000\n" {
		t.Fatal("wrong credential permissions")
	}
	if err = Inject(ctx, id, "1000:1000", []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	out, err = dockerx.Run(ctx, "exec", id, "cat", ConfigDir+"/hosts.yml")
	if err != nil || strings.Contains(string(out), "cxz-fixture") {
		t.Fatal("old credentials not cleared")
	}
}
