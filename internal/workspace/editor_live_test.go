package workspace

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/editor"
)

// Opt-in because the pinned IDE download and Linux container are external resources.
// This exercises the actual Manager path, not the browser fixture's startup stub.
func TestEditorDockerRemoteUserOwnershipAndTunnel(t *testing.T) {
	if os.Getenv("CXZ_TEST_EDITOR") != "1" {
		t.Skip("set CXZ_TEST_EDITOR=1 and CXZ_TEST_BINARY for a disposable editor container")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	root := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	m.Owner = core.ID()
	p := &Project{ID: core.ID(), Workspace: "/workspace", RemoteWorkspace: "/workspace", RemoteUser: "1000"}
	name := "cxz-editor-test-" + p.ID
	image := os.Getenv("CXZ_TEST_EDITOR_IMAGE")
	if image == "" {
		image = "debian:bookworm-slim"
	}
	if _, err = dockerx.Run(ctx, "run", "-d", "--name", name, "--label", "cxz.owner="+m.Owner, "--label", "cxz.project="+p.ID, image, "sleep", "infinity"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := dockerx.Run(ctx, "rm", "-f", name); err != nil {
			t.Error(err)
		}
	})
	p.ContainerID = name
	if _, err = dockerx.Run(ctx, "exec", name, "sh", "-c", "useradd -m -u 1000 editor && mkdir -p /workspace /cxz/tools && printf '# Editor probe\n' > /workspace/README.md && chown -R 1000:1000 /workspace"); err != nil {
		t.Fatal(err)
	}
	binary, err := filepath.Abs(os.Getenv("CXZ_TEST_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "cp", binary, name+":/cxz/tools/cxz"); err != nil {
		t.Fatal(err)
	}
	if archive := os.Getenv("CXZ_TEST_EDITOR_ARCHIVE"); archive != "" {
		src, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		cache := filepath.Join(root, "editor-cache")
		if err = os.MkdirAll(cache, 0700); err != nil {
			t.Fatal(err)
		}
		dst, err := os.Create(filepath.Join(cache, editor.Version+"-"+runtime.GOARCH+".tar.gz"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(dst, src)
		closeErr := dst.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	result, err := m.Editor(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Workspace != "/workspace" || len(result.Token) != 64 {
		t.Fatal("invalid editor workspace/token")
	}
	// Docker exec inherits HOME=/root here. Installation must use the passwd home.
	if _, err = dockerx.Run(ctx, "exec", name, "test", "-x", "/home/editor/.cache/cxz-editor/"+editor.Version+"/bin/openvscode-server"); err != nil {
		t.Fatal("remote-user home not used", err)
	}
	again, err := m.Editor(ctx, p.ID)
	if err != nil || again.Token != result.Token {
		t.Fatal("repeated Connect did not reuse the private server", err)
	}
	tunnel, err := m.OpenEditorTunnel(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := editor.BasePath(p.ID)
	if _, err = fmt.Fprintf(tunnel, "GET %s/ HTTP/1.1\r\nHost: 127.0.0.1:7351\r\nCookie: vscode-tkn=%s\r\nConnection: close\r\n\r\n", base, result.Token); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(tunnel), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	tunnel.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "vscode-workbench-web-configuration") {
		t.Fatalf("workbench tunnel: status %d, err %v", response.StatusCode, err)
	}
	owner := m.Owner
	m.Owner = "foreign-owner"
	if _, err = m.Editor(ctx, p.ID); err == nil {
		t.Fatal("foreign container accepted")
	}
	if _, err = m.OpenEditorTunnel(ctx, p.ID); err == nil {
		t.Fatal("foreign tunnel accepted")
	}
	m.Owner = owner
	if _, err = dockerx.Run(ctx, "stop", name); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Editor(ctx, p.ID); err == nil {
		t.Fatal("stopped container accepted")
	}
}
