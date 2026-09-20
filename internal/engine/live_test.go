package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"os"
	"testing"
	"time"
)

func TestLiveManagedEngine(t *testing.T) {
	if os.Getenv("CXZ_TEST_DIND") != "1" {
		t.Skip("set CXZ_TEST_DIND=1 for a disposable Docker-in-Docker lifecycle test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	e := Engine{Root: t.TempDir(), Owner: core.ID()}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.Down(ctx); err != nil {
			t.Error(err)
		}
		for _, r := range []struct{ kind, suffix string }{{"network", "-project"}, {"network", "-net"}, {"volume", "-data"}} {
			if _, err := dockerx.Run(ctx, r.kind, "rm", e.Name()+r.suffix); err != nil {
				t.Error(err)
			}
		}
	})
	spec := Spec{Mode: "dind", Image: "docker:29-dind"}
	if err := e.Ensure(ctx, spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "network", "create", "--label", "cxz.owner="+e.Owner, e.Name()+"-project"); err != nil {
		t.Fatal(err)
	}
	if err := e.Connect(ctx, e.Name()+"-project"); err != nil {
		t.Fatal(err)
	}
	if err := e.Ensure(ctx, spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "run", "--rm", "--network", e.Name()+"-project", "-e", "DOCKER_HOST="+e.Endpoint(), "docker:cli", "info"); err != nil {
		t.Fatal("project-side TCP access", err)
	}
	if _, err := dockerx.Run(ctx, "exec", e.Name(), "docker", "volume", "create", "cxz-test-cache"); err != nil {
		t.Fatal(err)
	}
	// Build an isolated scratch image to populate the disposable engine's cache.
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for name, body := range map[string]string{"Dockerfile": "FROM scratch\nCOPY fixture /fixture\n", "fixture": "cxz test"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dockerx.Input(ctx, &archive, "exec", "-i", e.Name(), "docker", "build", "-t", "cxz-cache-test", "-"); err != nil {
		t.Fatal(err)
	}
	info, err := e.Info(ctx)
	if err != nil || info.State != "running" || info.BuildCache == "" {
		t.Fatal(info, err)
	}
	if _, err := e.PruneBuildCache(ctx); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []struct{ kind, name string }{{"image", "cxz-cache-test"}, {"volume", "cxz-test-cache"}} {
		if _, err := dockerx.Run(ctx, "exec", e.Name(), "docker", resource.kind, "inspect", resource.name); err != nil {
			t.Fatal("prune removed retained resource", err)
		}
	}
	if err := e.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.Ensure(ctx, spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "exec", e.Name(), "docker", "volume", "inspect", "cxz-test-cache"); err != nil {
		t.Fatal("engine cache lost on recreation", err)
	}
}
