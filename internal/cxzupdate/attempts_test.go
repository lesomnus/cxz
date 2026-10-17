package cxzupdate

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestAttemptsKeepTheMostRecentAndSurviveACorruptHistory(t *testing.T) {
	root := t.TempDir()
	if a, ok := LastAttempt(root); ok {
		t.Fatal("attempt out of an empty history", a)
	}
	for i := 0; i < attemptHistory+10; i++ {
		if e := RecordAttempt(root, Attempt{At: time.Now(), Lease: strconv.Itoa(i), Outcome: "refused"}); e != nil {
			t.Fatal(e)
		}
	}
	all, e := Attempts(root)
	if e != nil {
		t.Fatal(e)
	}
	if len(all) != attemptHistory || all[0].Lease != "10" {
		t.Fatal("history not bounded to the most recent", len(all), all[0].Lease)
	}
	if a, _ := LastAttempt(root); a.Lease != strconv.Itoa(attemptHistory+9) {
		t.Fatal("last attempt is not the newest", a.Lease)
	}
	if e = os.WriteFile(AttemptsPath(root), []byte("{not json"), 0o600); e != nil {
		t.Fatal(e)
	}
	if e = RecordAttempt(root, Attempt{At: time.Now(), Outcome: "granted"}); e != nil {
		t.Fatal("a corrupt history blocked recording:", e)
	}
	if a, ok := LastAttempt(root); !ok || a.Outcome != "granted" {
		t.Fatal("recording did not replace the corrupt history", a)
	}
}

// A refusal holds the rollout off for RefusedInterval; anything else does not.
func TestRefusedHoldsOffOnlyWhileRecent(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if _, refused := Refused(root, now); refused {
		t.Fatal("empty history refused")
	}
	if e := RecordAttempt(root, Attempt{At: now, Outcome: "refused", Reason: "session s: busy"}); e != nil {
		t.Fatal(e)
	}
	a, refused := Refused(root, now.Add(RefusedInterval-time.Second))
	if !refused || a.Reason != "session s: busy" {
		t.Fatal("recent refusal not honored", a, refused)
	}
	if _, refused = Refused(root, now.Add(RefusedInterval)); refused {
		t.Fatal("refusal outlived its interval")
	}
	if e := RecordAttempt(root, Attempt{At: now, Outcome: "granted"}); e != nil {
		t.Fatal(e)
	}
	if _, refused = Refused(root, now); refused {
		t.Fatal("a granted attempt held the rollout off")
	}
}

func TestRecordAttemptTruncatesAnOversizedReason(t *testing.T) {
	root := t.TempDir()
	long := ""
	for len(long) < reasonLimit*2 {
		long += "한글 reason "
	}
	if e := RecordAttempt(root, Attempt{At: time.Now(), Outcome: "refused", Reason: long}); e != nil {
		t.Fatal(e)
	}
	a, _ := LastAttempt(root)
	if len(a.Reason) > reasonLimit {
		t.Fatal("reason not truncated", len(a.Reason))
	}
	if a.Reason == "" {
		t.Fatal("truncation emptied the reason")
	}
}
