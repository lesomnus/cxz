package containerterm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func TestDockerWisp(t *testing.T) {
	if os.Getenv("CXZ_TERMINAL_DOCKER_TEST") != "1" {
		t.Skip("opt-in Docker fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bin := filepath.Join(t.TempDir(), "cxz")
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
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if _, err := dockerx.Owned(cleanup, id, owner, project); err == nil {
			_, _ = dockerx.Run(cleanup, "rm", "-f", id)
		}
	}()
	if _, err := dockerx.Run(ctx, "exec", id, "mkdir", "-p", "/cxz/tools"); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "cp", bin, id+":/cxz/tools/cxz"); err != nil {
		t.Fatal(err)
	}
	p := &api.Project{Id: project, ContainerId: id, RemoteUser: "root"}
	pool := &WispPool{}
	defer pool.Close()
	start := time.Now()
	if _, err := ListPaths(ctx, p, "/"); err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy inspect+exec: %s", time.Since(start))
	for i := 0; i < 4; i++ {
		start = time.Now()
		out, err := pool.Paths(ctx, ctx, p, "/", nil)
		if err != nil || len(out.Entries) == 0 {
			t.Fatal(out, err)
		}
		t.Logf("wisp lookup %d: %s", i, time.Since(start))
	}
	key := p.Id + "/" + p.ContainerId + "/" + p.RemoteUser
	first := pool.clients[key]
	first.stop()
	select {
	case <-first.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := pool.Paths(ctx, ctx, p, "~/", nil); err != nil {
		t.Fatal("reconnect", err)
	}
	if pool.clients[key] == first {
		t.Fatal("dead connection reused")
	}
	p.RemoteUser = "65534"
	if _, err := pool.Paths(ctx, ctx, p, "/root", nil); err == nil {
		t.Fatal("nonroot accessed root home")
	}
	if _, err := pool.Paths(ctx, ctx, p, "/tmp", nil); err != nil {
		t.Fatal("nonroot lookup", err)
	}
	p.Id = "wrong"
	if _, err := pool.Paths(ctx, ctx, p, "/", nil); err == nil {
		t.Fatal("unowned accepted")
	}
}
