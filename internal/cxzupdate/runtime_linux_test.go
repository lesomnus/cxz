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
			root := os.Args[2]
			if os.Getenv("CXZ_TEST_RUNTIME_FAIL") == "1" && Revision == strings.Repeat("b", 40) {
				// Model an agent still working when the replacement crashes.
				_ = os.WriteFile(filepath.Join(root, "fixture-busy"), nil, 0600)
				os.Exit(42)
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
			defer cancel()
			var c io.Closer
			c, e = Maintenance(ctx, root, NewGate(root), func(context.Context) (map[string]string, error) {
				runs := map[string]string{"session": "unchanged-run"}
				if b, err := os.ReadFile(filepath.Join(root, "fixture-sessions.json")); err == nil {
					if err = json.Unmarshal(b, &runs); err != nil {
						return nil, err
					}
				}
				if _, err := os.Stat(filepath.Join(root, "fixture-busy")); err == nil {
					return runs, fmt.Errorf("session is working")
				}
				return runs, nil
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
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
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
	if h.Build.Revision != strings.Repeat("a", 40) || h.Sessions["session"] != "unchanged-run" || h.Lease != "" || h.Ready {
		t.Fatalf("rollback not healthy: %+v", h)
	}
	if e = ReplaceRuntime(ctx, root, target, r); e == nil {
		t.Fatal("failed revision retried immediately")
	}
}
func prepareRuntimeFixture(t *testing.T, root, target string, r Release) RuntimeTransaction {
	t.Helper()
	id := core.ID()
	h, e := Call(t.Context(), root, "prepare", id)
	if e != nil {
		t.Fatal(e)
	}
	tx := RuntimeTransaction{ID: id, Target: target, Revision: r.Revision, Release: &r, Old: h, State: "prepared"}
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		t.Fatal(e)
	}
	return tx
}

func fixtureBusy(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "fixture-busy"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePreparedRecoveryRestoresOldVersionWhileBusy(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	agent := exec.Command("sleep", "120")
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agent.Process.Kill(); _ = agent.Wait() }()
	before, err := ProcessStart(agent.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	fixtureBusy(t, root)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if e := stopRuntime(ctx, tx.Old); e != nil {
		t.Fatal(e)
	}
	if e := RecoverRuntime(ctx, root); e != nil {
		t.Fatal(e)
	}
	h, err := Call(ctx, root, "status", "")
	if err != nil {
		t.Fatal(err)
	}
	if h.Build.Revision != tx.Old.Build.Revision || h.Ready || h.Lease != "" || h.Sessions["session"] != "unchanged-run" {
		t.Fatalf("recovery installed a new version or lost the working session: %+v", h)
	}
	after, err := readRuntimeTransaction(root)
	if err != nil || after.State != "rolled_back" || after.Error == "" {
		t.Fatalf("recovery did not finish rollback: %+v, %v", after, err)
	}
	if after, err := ProcessStart(agent.Process.Pid); err != nil || after != before {
		t.Fatal("agent was restarted")
	}
}

func TestRuntimePreparedRecoveryWaitsForIdle(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	fixtureBusy(t, root)
	if err := RecoverRuntime(t.Context(), root); err == nil || !strings.Contains(err.Error(), "working") {
		t.Fatalf("working session did not veto forward recovery: %v", err)
	}
	h, err := Call(t.Context(), root, "status", "")
	if err != nil || h.Build.PID != tx.Old.Build.PID || h.Start != tx.Old.Start || h.Lease != "" {
		t.Fatalf("refusal restarted the old runtime or left admission closed: %+v, %v", h, err)
	}
	if err := os.Remove(filepath.Join(root, "fixture-busy")); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(t.Context(), root); err != nil {
		t.Fatal("did not resume when idle", err)
	}
	h, err = Call(t.Context(), root, "status", "")
	if err != nil || h.Build.Revision != r.Revision || h.Lease != "" {
		t.Fatalf("idle recovery failed: %+v, %v", h, err)
	}
}

func TestRuntimePreparedRecoveryRefreshesIdleSessionSnapshot(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	// While the helper was absent, admission expired and sessions changed.
	if _, err := Call(t.Context(), root, "release", tx.ID); err != nil {
		t.Fatal(err)
	}
	runs := map[string]string{"session": "resumed-run", "new-session": "new-run"}
	if err := core.WriteJSON(filepath.Join(root, "fixture-sessions.json"), runs); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(t.Context(), root); err != nil {
		t.Fatal("fresh idle proof did not refresh the snapshot", err)
	}
	h, err := Call(t.Context(), root, "status", "")
	if err != nil || h.Sessions["session"] != "resumed-run" || h.Sessions["new-session"] != "new-run" {
		t.Fatalf("session runs changed during replacement: %+v, %v", h, err)
	}
}

func TestRuntimeStartingRecoveryFinalizesHealthyTargetWhileBusy(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	tx.State = "starting"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := startRuntime(root, target, target, tx.ID); err != nil {
		t.Fatal(err)
	}
	before, err := waitHealth(ctx, root, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	fixtureBusy(t, root)
	if err := RecoverRuntime(ctx, root); err != nil {
		t.Fatal("finalization incorrectly required idle", err)
	}
	h, err := Call(ctx, root, "status", "")
	if err != nil || h.Build.PID != before.Build.PID || h.Start != before.Start || h.Ready || h.Lease != "" {
		t.Fatalf("finalization restarted the target or left admission closed: %+v, %v", h, err)
	}
	finished, err := readRuntimeTransaction(root)
	if err != nil || finished.State != "healthy" {
		t.Fatalf("target not finalized: %+v, %v", finished, err)
	}
}

func TestRuntimeRecoveryKeepsCapturedTargetWhenUpstreamMoves(t *testing.T) {
	root, target, r := runtimeFixture(t)
	prepareRuntimeFixture(t, root, target, r)
	upstream := r
	upstream.Revision = strings.Repeat("c", 40)
	// The newly published target need not even be staged to finish this update.
	if err := ReplaceRuntime(t.Context(), root, filepath.Join(root, "not-staged"), upstream); err != nil {
		t.Fatal(err)
	}
	h, err := Call(t.Context(), root, "status", "")
	if err != nil || h.Build.Revision != r.Revision {
		t.Fatalf("upstream retargeted recovery: %+v, %v", h, err)
	}
}

func TestRuntimeRecoveryDoesNotCreateTransaction(t *testing.T) {
	root, _, _ := runtimeFixture(t)
	h, err := Call(t.Context(), root, "health", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(t.Context(), root); err == nil || !strings.Contains(err.Error(), "no pending") {
		t.Fatalf("recovery created a fresh update: %v", err)
	}
	after, err := Call(t.Context(), root, "health", "")
	if err != nil || after.Build.PID != h.Build.PID || after.Start != h.Start {
		t.Fatalf("recovery changed the runtime: %+v, %v", after, err)
	}
}

func TestRuntimeRecoveryRefusesExpiredLeaseWhenOldRuntimeIsGone(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(root, "run", "update-lease.json"), lease{ID: tx.ID, Until: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(ctx, root); err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("recovery ignored expired admission: %v", err)
	}
	after, err := readRuntimeTransaction(root)
	if err != nil || after.State != "prepared" || after.New.Build.PID != 0 {
		t.Fatalf("recovery started a runtime without admission: %+v, %v", after, err)
	}
}

func TestRuntimePreparedRecoveryRejectsReusedPID(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	tx.Old.Build.PID = os.Getpid()
	tx.Old.Start = "not-the-recorded-process"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(ctx, root); err == nil || !strings.Contains(err.Error(), "PID was reused") {
		t.Fatalf("recovery ignored the process start identity: %v", err)
	}
	after, err := readRuntimeTransaction(root)
	if err != nil || after.State != "prepared" || after.New.Build.PID != 0 {
		t.Fatalf("recovery started a runtime with ambiguous identity: %+v, %v", after, err)
	}
}

func TestRuntimeRecoveryAcceptsLegacyTransaction(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	tx.Release = nil
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceRuntime(t.Context(), root, target, r); err != nil {
		t.Fatal(err)
	}
	after, err := readRuntimeTransaction(root)
	if err != nil || after.State != "healthy" || after.Release == nil || after.Release.Revision != r.Revision {
		t.Fatalf("legacy transaction was not upgraded: %+v, %v", after, err)
	}
}

func TestRuntimeRollbackRecoversDeadRecordedChildWhileBusy(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	// The recorded rollback child died before exposing maintenance health.
	tx.State, tx.New = "rolling_back", tx.Old
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	fixtureBusy(t, root)
	if err := RecoverRuntime(ctx, root); err != nil {
		t.Fatal(err)
	}
	h, err := Call(ctx, root, "status", "")
	if err != nil || h.Build.Revision != tx.Old.Build.Revision || h.Build.PID == tx.New.Build.PID || h.Ready || h.Lease != "" {
		t.Fatalf("rollback did not restore availability while busy: %+v, %v", h, err)
	}
}

func TestRuntimeRecoveryRefusesForeignLease(t *testing.T) {
	root, target, r := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, r)
	if _, err := Call(t.Context(), root, "release", tx.ID); err != nil {
		t.Fatal(err)
	}
	other := core.ID()
	if _, err := Call(t.Context(), root, "prepare", other); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRuntime(t.Context(), root); err == nil {
		t.Fatal("recovery ignored a foreign lease")
	}
	h, err := Call(t.Context(), root, "health", "")
	if err != nil || h.Build.PID != tx.Old.Build.PID || h.Start != tx.Old.Start || h.Lease != other {
		t.Fatalf("recovery changed the foreign maintenance operation: %+v, %v", h, err)
	}
}
func TestStopRuntimeRejectsReusedPID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := stopRuntime(ctx, Health{Build: Build{PID: os.Getpid()}, Start: "wrong-start"}); e == nil {
		t.Fatal("accepted mismatched process identity")
	}
}
