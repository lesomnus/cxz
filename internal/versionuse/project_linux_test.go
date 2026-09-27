package versionuse

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessIdentityCannotTargetUnrelatedProcesses(t *testing.T) {
	p, e := process(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	p.Start = "wrong"
	if e = stop(context.Background(), p); e == nil {
		t.Fatal("reused PID accepted")
	}
	for _, args := range [][]string{{"cxz", "--state", "/other", "_project"}, {"sh", "-c", "cxz --state /state _project"}, {"cxz", "--state", "/state", "_guard", "123"}} {
		if matches(args, "/state", "_project") {
			t.Fatal(args)
		}
	}
}
func TestForceProjectReleaseSwitchDocker(t *testing.T) {
	if os.Getenv("CXZ_TEST_USE_DOCKER") != "1" {
		t.Skip("set CXZ_TEST_USE_DOCKER=1 for isolated process/release switch verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	repo, _ := filepath.Abs("../..")
	dir := t.TempDir()
	for _, build := range []struct{ name, pkg, version string }{{"new", "./cmd/cxz", "v9.0.0"}, {"old", "./cmd/cxz", "v0.1.0"}, {"fake", "./internal/testagent", ""}, {"driver", "./internal/testuse", ""}} {
		args := []string{"build", "-o", filepath.Join(dir, build.name)}
		if build.version != "" {
			args = append(args, "-ldflags=-X main.version="+build.version)
		}
		args = append(args, build.pkg)
		c := exec.CommandContext(ctx, "go", args...)
		c.Dir = repo
		c.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatal(string(b), e)
		}
	}
	name := "cxz-use-test-" + core.ID()
	run := func(args ...string) string {
		t.Helper()
		b, e := dockerx.Run(ctx, args...)
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(string(b))
	}
	id := run("run", "-d", "--name", name, "alpine:3.22", "sleep", "180")
	defer dockerx.Run(context.Background(), "rm", "-f", id)
	run("exec", id, "sh", "-c", "mkdir -p /test /work /cxz/state/data; chmod 700 /cxz/state/data")
	for _, bin := range []string{"new", "old", "fake", "driver"} {
		run("cp", filepath.Join(dir, bin), id+":/test/"+bin)
	}
	runtime := filepath.Join(dir, "runtime.json")
	os.WriteFile(runtime, []byte(`{"ProjectID":"aaaaaaaaaaaaaaaaaaaaaaaa","Workspace":"/work","Token":"fixture","Claude":"/test/fake"}`), 0600)
	run("cp", runtime, id+":/cxz/state/runtime.json")
	run("exec", id, "/test/new", "--state", "/cxz/state/data", "_boot")
	run("exec", id, "/test/driver", "init")
	gen := core.ID()
	args := []string{"exec", id, "/test/old", "_use-project", "stop", "/cxz/state/data", gen, "v0.1.0"}
	run(args...)
	run("exec", id, "/test/old", "_use-project", "resume", "/cxz/state/data", gen, "v0.1.0")
	first := run("exec", id, "/test/driver", "check")
	// A lost final acknowledgement must not restart an already restored session.
	run(args...)
	run("exec", id, "/test/old", "_use-project", "resume", "/cxz/state/data", gen, "v0.1.0")
	if second := run("exec", id, "/test/driver", "check"); second != first {
		t.Fatal("duplicate switch restarted the run", first, second)
	}
	after, e := dockerx.Inspect(ctx, id)
	if e != nil || after.ID != id || !after.State.Running {
		t.Fatal("project container replaced", e)
	}
	// The running supervisor itself (not the mutable tools filename) is v0.1.0.
	var result map[string]any
	if e = json.Unmarshal([]byte(first), &result); e != nil {
		t.Fatal(e)
	}
	actual := run("exec", id, fmt.Sprintf("/proc/%.0f/exe", result["supervisor"]), "--format", "json", "version")
	if !strings.Contains(actual, `"version":"v0.1.0"`) && !strings.Contains(actual, `"version": "v0.1.0"`) {
		t.Fatal("wrong supervisor release", actual)
	}

	t.Log("working agent forced to a new run; account, vendor conversation, model and single input preserved; stopped session and container unchanged")
}
