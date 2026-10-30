package cxzupdate

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func TestRecoveryBackoffPersistsAndBoundsDispatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	o := RecoveryObservation{Project: "project", Container: "container", Transaction: "tx", Phase: "rolling_back"}
	calls := 0
	run := func(finalize bool) RecoveryResult {
		if finalize {
			t.Fatal("unhealthy runtime was sent to completion-only handling")
		}
		calls++
		// Admission is durable before staging or starting a helper.
		saved, err := LoadRecovery(path)
		if err != nil || saved.Attempts != calls || saved.Status != "running" || saved.NextAttempt.Sub(now) != 5*time.Minute {
			t.Fatalf("attempt was not saved before dispatch: %+v, %v", saved, err)
		}
		return RecoveryResult{Failure: RecoveryProblem(fmt.Errorf("temporary Docker failure"))}
	}
	for attempt := 1; attempt <= RecoveryAttempts; attempt++ {
		r, ran, err := RunRecovery(t.Context(), path, o, clock, run)
		if err != nil || !ran || r.Attempts != attempt {
			t.Fatalf("attempt %d: %+v, %v, %v", attempt, r, ran, err)
		}
		if attempt == RecoveryAttempts {
			if r.Status != "stopped" || r.Code != "attempt_limit" || !r.NextAttempt.IsZero() {
				t.Fatalf("budget did not stop: %+v", r)
			}
			break
		}
		want := 30 * time.Second * time.Duration(1<<(attempt-1))
		if r.NextAttempt.Sub(now) != want {
			t.Fatalf("attempt %d waits %s, want %s", attempt, r.NextAttempt.Sub(now), want)
		}
		now = r.NextAttempt.Add(-time.Nanosecond)
		_, ran, err = RunRecovery(t.Context(), path, o, clock, run)
		if err != nil || ran || calls != attempt {
			t.Fatal("a poll before the deadline dispatched work", ran, err, calls)
		}
		now = now.Add(time.Nanosecond)
	}
	now = now.Add(24 * time.Hour)
	_, ran, err := RunRecovery(t.Context(), path, o, clock, run)
	if err != nil || ran || calls != RecoveryAttempts {
		t.Fatal("stopped recovery restarted automatically", ran, err, calls)
	}
}

func TestRecoveryInterruptedManagerRetainsAttemptAndAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	now := time.Now()
	o := RecoveryObservation{Project: "p", Container: "c", Transaction: "tx"}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("fixture did not interrupt the Manager")
			}
		}()
		_, _, _ = RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
			panic("Manager terminated after dispatch")
		})
	}()
	now = now.Add(4 * time.Minute)
	_, ran, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
		t.Fatal("interrupted helper was duplicated before its deadline")
		return RecoveryResult{}
	})
	if err != nil || ran {
		t.Fatal(ran, err)
	}
	now = now.Add(time.Minute)
	r, ran, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
		return RecoveryResult{Failure: RecoveryProblem(fmt.Errorf("still unavailable"))}
	})
	if err != nil || !ran || r.Attempts != 2 {
		t.Fatalf("restart reset the budget: %+v, %v", r, err)
	}
}

func TestRecoveryWaitingDoesNotSpendBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	now := time.Now()
	o := RecoveryObservation{Project: "p", Container: "c", Transaction: "tx", Phase: "prepared", Available: true}
	r, ran, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
		return RecoveryResult{Available: true, Failure: RecoveryProblem(RecoveryWaiting("not_idle", "agent working"))}
	})
	if err != nil || !ran || r.Attempts != 0 || r.Status != "waiting" {
		t.Fatalf("busy readiness spent the failure budget: %+v, %v", r, err)
	}
	o.Waiting, o.Reason = true, "agent working"
	for range 20 {
		_, ran, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
			t.Fatal("readiness waiting dispatched a helper")
			return RecoveryResult{}
		})
		if err != nil || ran {
			t.Fatal(ran, err)
		}
	}
	o.Waiting, o.Idle = false, true
	r, ran, err = RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult { return RecoveryResult{Available: true} })
	if err != nil || !ran || r.Attempts != 1 || r.Status != "completed" {
		t.Fatalf("fresh idle observation did not resume: %+v, %v", r, err)
	}
}

func TestRecoveryStopsOperatorErrorsAndUnsafeLeaseWindow(t *testing.T) {
	for _, permanent := range []bool{true, false} {
		t.Run(fmt.Sprint(permanent), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recovery.json")
			now := time.Now()
			o := RecoveryObservation{Project: "p", Container: "c", Transaction: "tx", Phase: "rolling_back", LeaseUntil: now.Add(45 * time.Second)}
			r, _, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult {
				if permanent {
					return RecoveryResult{Failure: RecoveryProblem(RecoveryBlocked("lease_expired", "maintenance lease expired"))}
				}
				return RecoveryResult{Failure: RecoveryProblem(fmt.Errorf("temporary timeout"))}
			})
			if err != nil || r.Status != "stopped" || r.Attempts != 1 {
				t.Fatalf("unsafe recovery was not stopped: %+v, %v", r, err)
			}
			o.Waiting = true
			_, ran, err := RunRecovery(t.Context(), path, o, func() time.Time { return now }, func(bool) RecoveryResult { t.Fatal("stopped state bypassed"); return RecoveryResult{} })
			if err != nil || ran {
				t.Fatal(ran, err)
			}
		})
	}
}

func TestRecoveryBudgetFollowsTransactionAndContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	r := Recovery{Project: "p", Container: "c", Transaction: "old", Status: "stopped", Attempts: RecoveryAttempts}
	if err := core.WriteJSON(path, r); err != nil {
		t.Fatal(err)
	}
	o := RecoveryObservation{Project: "p", Container: "c", Transaction: "new"}
	r, ran, err := RunRecovery(context.Background(), path, o, time.Now, func(bool) RecoveryResult { return RecoveryResult{} })
	if err != nil || !ran || r.Attempts != 1 || r.Transaction != "new" {
		t.Fatalf("new transaction inherited the old budget: %+v, %v", r, err)
	}
	o.Container = "recreated"
	r, ran, err = RunRecovery(context.Background(), path, o, time.Now, func(bool) RecoveryResult { return RecoveryResult{} })
	if err != nil || !ran || r.Attempts != 1 || r.Container != "recreated" {
		t.Fatalf("recreated container inherited the old budget: %+v, %v", r, err)
	}
}
