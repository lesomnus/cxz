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
	hostSecretRoot := "/dev/shm/cxz-wisp-test-" + owner
	b, err := dockerx.Run(ctx, "run", "-d", "-e", "CXZ_SECRET_STORAGE=host-tmpfs", "-v", hostSecretRoot+":/cxz/secrets", "--label", "cxz.owner="+owner, "--label", "cxz.project="+project, image, "sleep", "90")
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
	if _, err := dockerx.Run(ctx, "exec", id, "chmod", "1777", "/cxz/secrets"); err != nil {
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
	secretPath, err := pool.PutSecret(ctx, ctx, p, "session/run", []byte("fixture-secret\nsecond line"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := dockerx.Run(ctx, "exec", "--user", "65534", id, "cat", secretPath)
	if err != nil || string(data) != "fixture-secret\nsecond line" {
		t.Fatal("secret content mismatch")
	}
	mode, err := dockerx.Run(ctx, "exec", id, "stat", "-c", "%a", secretPath)
	if err != nil || strings.TrimSpace(string(mode)) != "600" {
		t.Fatal("secret file permissions", err)
	}
	mode, err = dockerx.Run(ctx, "exec", id, "stat", "-c", "%a", filepath.Dir(secretPath))
	if err != nil || strings.TrimSpace(string(mode)) != "700" {
		t.Fatal("secret directory permissions", err)
	}
	if err = pool.DeleteSecret(ctx, ctx, p, secretPath); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", id, "test", "-e", secretPath); err == nil {
		t.Fatal("secret not deleted")
	}
	if _, err := pool.Paths(ctx, ctx, p, "/root", nil); err == nil {
		t.Fatal("nonroot accessed root home")
	}
	if _, err := pool.Paths(ctx, ctx, p, "/tmp", nil); err != nil {
		t.Fatal("nonroot lookup", err)
	}
	secretPath, err = pool.PutSecret(ctx, ctx, p, "session/disconnect", []byte("ephemeral"))
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if len(filepath.Base(filepath.Dir(secretPath))) != 8 {
		t.Fatal("secret name is not 8 characters")
	}
	if _, err = dockerx.Run(ctx, "stop", id); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+owner, "-v", hostSecretRoot+":/cxz/secrets", image, "test", "-f", secretPath); err != nil {
		t.Fatal("secret lost after all containers stopped", err)
	}
	if _, err = dockerx.Run(ctx, "start", id); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", id, "rm", secretPath); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", id, "rmdir", filepath.Dir(secretPath)); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "run", "--rm", "--label", "cxz.owner="+owner, "-v", "/dev/shm:/host-shm", image, "rmdir", "/host-shm/"+filepath.Base(hostSecretRoot)); err != nil {
		t.Fatal(err)
	}
	p.Id = "wrong"
	if _, err := pool.Paths(ctx, ctx, p, "/", nil); err == nil {
		t.Fatal("unowned accepted")
	}
}
