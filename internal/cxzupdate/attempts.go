package cxzupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// An update attempt used to leave no trace. The helper printed why it stopped
// to its own stdout and was removed before the next tick, so a rollout that
// could not succeed retried every 30 seconds for hours and the reason had to be
// read out of a container that no longer existed. Record the attempt where the
// decision is made -- next to the lease -- and the installation can say why it
// is not updating, and wait before asking again. See cxz#210.
type Attempt struct {
	At      time.Time `json:"at"`
	Lease   string    `json:"lease,omitempty"`
	Outcome string    `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
}

var (
	// RefusedInterval is how long the rollout waits after being refused. A
	// refusal is not a failure -- the server was busy, and will not be busy
	// forever -- so it does not earn the failure backoff. But every attempt
	// asks every session whether it is quiet, which is not free.
	RefusedInterval = Tunable("CXZ_UPDATE_REFUSED_INTERVAL", 10*time.Minute)
)

const (
	attemptHistory = 50
	reasonLimit    = 500
)

func AttemptsPath(root string) string {
	return filepath.Join(root, "cxz-update-attempts.json")
}
func Attempts(root string) ([]Attempt, error) {
	b, e := os.ReadFile(AttemptsPath(root))
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []Attempt
	if e = json.Unmarshal(b, &out); e != nil {
		return nil, e
	}
	return out, nil
}

// RecordAttempt keeps the most recent attemptHistory attempts. A history that
// cannot be read is replaced rather than propagated: it is a record of why an
// update did not happen, and must never be the reason one does not.
func RecordAttempt(root string, a Attempt) error {
	lock, e := core.Lock(filepath.Join(root, "update-attempts.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	all, _ := Attempts(root)
	if len(a.Reason) > reasonLimit {
		a.Reason = strings.ToValidUTF8(a.Reason[:reasonLimit], "")
	}
	all = append(all, a)
	if len(all) > attemptHistory {
		all = all[len(all)-attemptHistory:]
	}
	return core.WriteJSON(AttemptsPath(root), all)
}
func LastAttempt(root string) (Attempt, bool) {
	all, e := Attempts(root)
	if e != nil || len(all) == 0 {
		return Attempt{}, false
	}
	return all[len(all)-1], true
}

// Refused reports a refusal recent enough that asking again is only noise.
func Refused(root string, now time.Time) (Attempt, bool) {
	a, ok := LastAttempt(root)
	if !ok || a.Outcome != "refused" {
		return Attempt{}, false
	}
	return a, now.Sub(a.At) < RefusedInterval
}
