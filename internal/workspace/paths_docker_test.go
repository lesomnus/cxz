package workspace

import (
	"context"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDockerManagerPaths(t *testing.T) {
	if os.Getenv("CXZ_TERMINAL_DOCKER_TEST") != "1" {
		t.Skip("opt-in Docker fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	bin := filepath.Join(root, "cxz")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "../../cmd/cxz")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	owner, project := core.ID(), core.ID()
	image := os.Getenv("CXZ_TERMINAL_TEST_IMAGE")
	if image == "" {
		image = "alpine:latest"
	}
	b, err := dockerx.Run(ctx, "run", "-d", "--label", "cxz.owner="+owner, "--label", "cxz.project="+project, image, "sleep", "90")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(string(b))
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := dockerx.Owned(cleanup, id, owner, project); err == nil {
			_, _ = dockerx.Run(cleanup, "rm", "-f", id)
		}
	}()
	if _, err := dockerx.Run(ctx, "exec", id, "sh", "-c", "mkdir -p /cxz/tools /browse/'한글 dir'; adduser -D -u 1001 browse; touch /browse/run.sh; chmod 755 /browse/run.sh; ln -s '한글 dir' /browse/link; chmod 700 /root"); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "cp", bin, id+":/cxz/tools/cxz"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	t.Setenv("CXZ_MANAGER_CONTAINER", "")
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Owner = owner
	if err := m.save(ctx, &Project{ID: project, ContainerID: id, RemoteUser: "browse", RemoteWorkspace: "/browse"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		partial := false
		out, err := m.Paths(ctx, project, "/browse/", func(containerterm.PathListing) { partial = true })
		if err != nil || len(out.Entries) != 3 || !partial {
			t.Fatal(out, err)
		}
		entries := map[string]containerterm.PathEntry{}
		for _, entry := range out.Entries {
			entries[entry.Name] = entry
		}
		if !entries["한글 dir"].Directory || !entries["link"].Symlink || entries["link"].LinkTarget != "한글 dir" || !entries["run.sh"].Executable {
			t.Fatal(entries)
		}
	}
	if _, err := m.Paths(ctx, project, "~/", nil); err != nil {
		t.Fatal("remote user's home unavailable", err)
	}
	if _, err := m.Paths(ctx, project, "/root/", nil); err == nil {
		t.Fatal("lookup ran as root instead of remote user")
	}
	terminal, err := m.OpenTerminal(ctx, project, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if _, err := io.WriteString(terminal, "printf 'user=%s\\ndir=%s\\n' \"$(id -u)\" \"$PWD\"; stty size; exit\r"); err != nil {
		t.Fatal(err)
	}
	output, _ := io.ReadAll(terminal) // A PTY may return EIO at normal shell exit.
	if err := terminal.Wait(); err != nil {
		t.Fatal(err, string(output))
	}
	for _, want := range []string{"user=1001", "dir=/browse", "24 80"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("terminal missing %q: %s", want, output)
		}
	}
	m.Owner = "foreign"
	if _, err := m.Paths(ctx, project, "/", nil); err == nil {
		t.Fatal("cached helper bypassed manager ownership")
	}
	if terminal, err := m.OpenTerminal(ctx, project, 80, 24); err == nil {
		terminal.Close()
		terminal.Wait()
		t.Fatal("terminal bypassed manager ownership")
	}
	m.Owner = owner
	if _, err := dockerx.Run(ctx, "stop", id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Paths(ctx, project, "/", nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stopped project lookup", err)
	}
	if _, err := m.OpenTerminal(ctx, project, 80, 24); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stopped project terminal", err)
	}
}
