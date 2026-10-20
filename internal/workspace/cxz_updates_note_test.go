package workspace

import (
	"os"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/cxzupdate"
)

// Every tick that stops early says where it stopped. A reason belonging to an
// earlier tick is worse than none: "another rollout holds the lock" stayed in
// the state file for an hour after the lock was free, and the afternoon went
// into /proc/locks instead of the transaction that was actually blocking.
// See cxz#222, cxz#253.
func TestATickThatStopsEarlySaysSo(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	if err := cxzupdate.Save(m.Root, cxzupdate.State{State: "holding", Reason: "another rollout holds the lock"}); err != nil {
		t.Fatal(err)
	}
	m.noteUpdateStop("waiting", "automatic updates are paused")
	s, err := cxzupdate.Load(m.Root)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != "waiting" || s.Reason != "automatic updates are paused" {
		t.Fatalf("the stale reason survived: %+v", s)
	}
	// Repeating itself every thirty seconds would rewrite the file forever, so
	// an unchanged answer is not written at all.
	info, err := os.Stat(cxzupdate.StatePath(m.Root))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	m.noteUpdateStop("waiting", "automatic updates are paused")
	again, err := os.Stat(cxzupdate.StatePath(m.Root))
	if err != nil {
		t.Fatal(err)
	}
	if !again.ModTime().Equal(info.ModTime()) {
		t.Fatal("an unchanged reason was written again")
	}
}
