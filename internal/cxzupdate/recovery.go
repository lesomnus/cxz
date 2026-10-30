package cxzupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

const RecoveryAttempts = 5

// RecoveryFailure travels over the private maintenance/helper protocols. A
// reason is for people; policy must not depend on matching its wording.
type RecoveryFailure struct {
	Class  string `json:"class"` // waiting, transient, operator_required
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

func (e *RecoveryFailure) Error() string { return e.Reason }
func RecoveryBlocked(code, reason string) error {
	return &RecoveryFailure{Class: "operator_required", Code: code, Reason: reason}
}
func RecoveryWaiting(code, reason string) error {
	return &RecoveryFailure{Class: "waiting", Code: code, Reason: reason}
}
func RecoveryProblem(err error) *RecoveryFailure {
	if err == nil {
		return nil
	}
	var failure *RecoveryFailure
	if errors.As(err, &failure) {
		return failure
	}
	return &RecoveryFailure{Class: "transient", Code: "recovery_failed", Reason: err.Error()}
}

type RecoveryResult struct {
	Outcome     string           `json:"outcome"`
	Transaction string           `json:"transaction,omitempty"`
	Phase       string           `json:"phase,omitempty"`
	Available   bool             `json:"runtime_available"`
	LeaseUntil  time.Time        `json:"lease_until,omitempty"`
	Failure     *RecoveryFailure `json:"failure,omitempty"`
}

// Recovery is the Manager's durable automatic retry budget, scoped to a
// project/container/transaction. It remains readable when the runtime is dead
// and survives Manager replacement, release checks, and channel changes.
type Recovery struct {
	Project      string           `json:"project"`
	Name         string           `json:"name,omitempty"`
	Container    string           `json:"container"`
	Transaction  string           `json:"transaction,omitempty"`
	Phase        string           `json:"phase,omitempty"`
	Available    bool             `json:"runtime_available"`
	Status       string           `json:"status"`
	Attempts     int              `json:"attempts"`
	MaxAttempts  int              `json:"max_attempts"`
	LastAttempt  time.Time        `json:"last_attempt,omitempty"`
	NextAttempt  time.Time        `json:"next_attempt,omitempty"`
	LeaseUntil   time.Time        `json:"lease_until,omitempty"`
	Code         string           `json:"code,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	FinalizeSeen string           `json:"finalize_seen,omitempty"`
	LastFailure  *RecoveryFailure `json:"last_failure,omitempty"`
}

type RecoveryObservation struct {
	Project, Name, Container, Transaction, Phase string
	Available, Idle, Waiting                     bool
	Reason                                       string
	LeaseUntil                                   time.Time
	// A positive PID/start/revision/lease match permits one completion-only
	// attempt per observed runtime identity, even after the repair budget stops.
	Finalize string
}

func LoadRecovery(path string) (Recovery, error) {
	var r Recovery
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	return r, err
}

func ReadRecoveries(root string) ([]Recovery, error) {
	paths, err := filepath.Glob(filepath.Join(root, "projects", "*", "runtime-recovery.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Recovery, 0, len(paths))
	for _, path := range paths {
		r, err := LoadRecovery(path)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// RunRecovery is called under rollout.lock. Persisting admission before run
// means a killed Manager cannot reset the budget or immediately launch another
// helper; that interrupted attempt remains counted after restart.
func RunRecovery(ctx context.Context, path string, o RecoveryObservation, now func() time.Time, run func(bool) RecoveryResult) (Recovery, bool, error) {
	r, err := LoadRecovery(path)
	if err != nil {
		return r, false, err
	}
	before := r
	if r.Project != o.Project || r.Container != o.Container || o.Transaction != "" && r.Transaction != "" && r.Transaction != o.Transaction {
		r = Recovery{Project: o.Project, Container: o.Container}
	}
	r.Name, r.Available = o.Name, o.Available
	r.MaxAttempts = RecoveryAttempts
	if o.Transaction != "" {
		r.Transaction, r.Phase = o.Transaction, o.Phase
	}
	if !o.LeaseUntil.IsZero() {
		r.LeaseUntil = o.LeaseUntil
	}
	save := func() error {
		if reflect.DeepEqual(before, r) {
			return nil
		}
		return core.WriteJSON(path, r)
	}
	if err := ctx.Err(); err != nil {
		return r, false, err
	}
	finalize := o.Finalize != "" && o.Finalize != r.FinalizeSeen
	// A new idle observation can end a normal readiness wait immediately, but
	// does not reset a failure's backoff or an exhausted budget.
	if !finalize && (r.Status == "stopped" || !r.NextAttempt.IsZero() && now().Before(r.NextAttempt) && !(r.Status == "waiting" && o.Idle)) {
		return r, false, save()
	}
	if o.Waiting && !finalize {
		r.Status, r.Code, r.Reason = "waiting", "not_idle", o.Reason
		return r, false, save()
	}
	if !finalize && r.Attempts >= RecoveryAttempts {
		r.stop("attempt_limit", "automatic runtime recovery exhausted; resolve the cause and retry explicitly")
		return r, false, save()
	}
	if !finalize && !r.LeaseUntil.IsZero() && !(r.Phase == "prepared" && r.Available) && !now().Before(r.LeaseUntil.Add(-30*time.Second)) {
		r.stop("lease_expiring", "no safe retry window remains before the maintenance lease expires")
		return r, false, save()
	}
	if finalize {
		r.FinalizeSeen = o.Finalize
	} else {
		r.Attempts++
	}
	r.Status, r.LastAttempt = "running", now()
	// The rollout pass has a five-minute context. Its lease on this attempt
	// prevents a restarted Manager from racing the still-running helper.
	r.NextAttempt = r.LastAttempt.Add(5 * time.Minute)
	if err := save(); err != nil {
		return r, false, err
	}
	// Even a subsequent budgeted retry must remain completion-only when the
	// observed runtime already has the expected identity.
	result := run(o.Finalize != "")
	if result.Transaction != "" {
		r.Transaction, r.Phase = result.Transaction, result.Phase
	}
	r.Available = result.Available
	if result.Transaction != "" {
		r.LeaseUntil = result.LeaseUntil
	}
	if result.Failure == nil {
		r.Status, r.Code, r.Reason = "completed", "", ""
		r.LastFailure = nil
		r.NextAttempt = time.Time{}
	} else {
		f := result.Failure
		r.Code, r.Reason = f.Code, f.Reason
		if len(r.Reason) > reasonLimit {
			r.Reason = string([]rune(r.Reason)[:min(reasonLimit, len([]rune(r.Reason)))])
		}
		if f.Class != "waiting" {
			r.LastFailure = &RecoveryFailure{Class: f.Class, Code: f.Code, Reason: r.Reason}
		}
		switch f.Class {
		case "operator_required":
			r.stop(f.Code, r.Reason)
		case "waiting":
			if !finalize {
				r.Attempts--
			}
			r.Status = "waiting"
			r.NextAttempt = now().Add(30 * time.Second)
			if f.Code == "not_idle" {
				r.NextAttempt = now().Add(RefusedInterval)
			}
		default:
			r.Status = "retrying"
			r.NextAttempt = now().Add(30 * time.Second * time.Duration(1<<min(max(r.Attempts-1, 0), 3)))
			if r.Attempts >= RecoveryAttempts {
				r.stop("attempt_limit", "automatic runtime recovery exhausted: "+r.Reason)
			} else if !r.LeaseUntil.IsZero() && !(r.Phase == "prepared" && r.Available) && !r.NextAttempt.Before(r.LeaseUntil.Add(-30*time.Second)) {
				r.stop("lease_expiring", "no safe retry window remains before the maintenance lease expires: "+r.Reason)
			}
		}
	}
	return r, true, core.WriteJSON(path, r)
}

func (r *Recovery) stop(code, reason string) {
	r.Status, r.Code, r.Reason, r.NextAttempt = "stopped", code, reason, time.Time{}
}

// ResetRecovery only resets scheduling. It never edits runtime transactions,
// maintenance leases, pinned binaries, or session identities.
func ResetRecovery(path string) (Recovery, error) {
	r, err := LoadRecovery(path)
	if err != nil {
		return r, err
	}
	if r.Project == "" {
		return r, fmt.Errorf("no runtime recovery recorded for this project")
	}
	r.Status, r.Attempts = "retrying", 0
	r.MaxAttempts = RecoveryAttempts
	r.Code, r.Reason, r.FinalizeSeen = "", "manual retry requested; safety checks remain required", ""
	r.NextAttempt = time.Time{}
	r.LeaseUntil = time.Time{} // Re-read real ownership; never reuse a stale cached deadline.
	return r, core.WriteJSON(path, r)
}
