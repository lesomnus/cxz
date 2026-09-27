package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("CXZ_TEST_DOCKER_STATE") != "" {
		if e := fixtureDocker(os.Args[1:]); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv("CXZ_TEST_RUNTIME_CHILD") == "1" {
		exe, _ := os.Executable()
		Revision = strings.Repeat("b", 40)
		if strings.Contains(exe, "/old") || strings.Contains(exe, strings.Repeat("a", 40)) {
			Revision = strings.Repeat("a", 40)
		}
		var e error
		switch {
		case len(os.Args) == 2 && os.Args[1] == "_build-info":
			e = json.NewEncoder(os.Stdout).Encode(Current())
		case len(os.Args) == 5 && os.Args[1] == "_update-runtime-start":
			e = RuntimeChild(os.Args[2], os.Args[3], os.Args[4])
		case len(os.Args) == 4 && os.Args[3] == "_project":
			if os.Getenv("CXZ_TEST_RUNTIME_FAIL") == "1" && Revision == strings.Repeat("b", 40) {
				os.Exit(42)
			}
			root := os.Args[2]
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
			defer cancel()
			var c io.Closer
			c, e = Maintenance(ctx, root, NewGate(root), func(context.Context) (map[string]string, error) {
				return map[string]string{"session": "unchanged-run"}, nil
			})
			if e == nil {
				<-ctx.Done()
				c.Close()
			}
		default:
			e = fmt.Errorf("unknown fixture command")
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func runtimeFixture(t *testing.T) (string, string, Release) {
	t.Helper()
	t.Setenv("CXZ_TEST_RUNTIME_CHILD", "1")
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	old, new := filepath.Join(root, "old"), filepath.Join(root, "candidate")
	for _, p := range []string{old, new} {
		if e = os.WriteFile(p, data, 0755); e != nil {
			t.Fatal(e)
		}
	}
	c := exec.Command(old, "--state", root, "_project")
	c.Stderr = os.Stderr
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	go c.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e = waitHealth(ctx, root, strings.Repeat("a", 40)); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if h, e := Call(ctx, root, "health", ""); e == nil {
			_ = stopRuntime(ctx, h)
		}
		_ = c.Process.Kill()
	})
	return root, new, testRelease(data)
}
func TestRuntimeReplacementPreservesUnrelatedAgent(t *testing.T) {
	root, target, r := runtimeFixture(t)
	agent := exec.Command("sleep", "120")
	if e := agent.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = agent.Process.Kill(); _ = agent.Wait() }()
	before, e := ProcessStart(agent.Process.Pid)
	if e != nil {
		t.Fatal(e)
	}
	old, e := Call(context.Background(), root, "health", "")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if e = ReplaceRuntime(ctx, root, target, r); e != nil {
		t.Fatal(e)
	}
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		t.Fatal(e)
	}
	if h.Build.Revision != r.Revision || h.Build.PID == old.Build.PID || h.Lease != "" || h.Sessions["session"] != "unchanged-run" {
		t.Fatalf("bad replacement: %+v", h)
	}
	if after, e := ProcessStart(agent.Process.Pid); e != nil || after != before {
		t.Fatal("agent was restarted")
	}
	if e = ReplaceRuntime(ctx, root, target, r); e != nil {
		t.Fatal("repeated acknowledgement", e)
	}
}
func TestRuntimeRollsBackFailureBeforeMaintenanceStarts(t *testing.T) {
	root, target, r := runtimeFixture(t)
	t.Setenv("CXZ_TEST_RUNTIME_FAIL", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	if e := ReplaceRuntime(ctx, root, target, r); e != nil {
		t.Fatal(e)
	}
	tx, e := readRuntimeTransaction(root)
	if e != nil {
		t.Fatal(e)
	}
	if tx.State != "rolled_back" || tx.FailedAt.IsZero() {
		t.Fatalf("bad transaction: %+v", tx)
	}
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		t.Fatal(e)
	}
	if h.Build.Revision != strings.Repeat("a", 40) || h.Sessions["session"] != "unchanged-run" || h.Lease != "" {
		t.Fatalf("rollback not healthy: %+v", h)
	}
	if e = ReplaceRuntime(ctx, root, target, r); e == nil {
		t.Fatal("failed revision retried immediately")
	}
}
func TestRuntimeRecoversPreparedTransaction(t *testing.T) {
	root, target, r := runtimeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	id := core.ID()
	h, e := Call(ctx, root, "prepare", id)
	if e != nil {
		t.Fatal(e)
	}
	tx := RuntimeTransaction{ID: id, Target: target, Revision: r.Revision, Old: h, State: "prepared"}
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		t.Fatal(e)
	}
	if e = stopRuntime(ctx, h); e != nil {
		t.Fatal(e)
	}
	if e = ReplaceRuntime(ctx, root, target, r); e != nil {
		t.Fatal(e)
	}
}
func TestStopRuntimeRejectsReusedPID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := stopRuntime(ctx, Health{Build: Build{PID: os.Getpid()}, Start: "wrong-start"}); e == nil {
		t.Fatal("accepted mismatched process identity")
	}
}
