package main

import (
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

// Whether a rollout is actually running is read, not taken. A status command
// that acquired the lock to find out would make the next tick report a holder
// that was the status command itself. See cxz#253.
func TestTheRolloutLockIsReadWithoutTakingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.lock")
	held, err := lockHeld(path)
	if err != nil || held {
		t.Fatal("a lock file that does not exist was reported held", held, err)
	}
	lock, err := core.Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	held, err = lockHeld(path)
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatal("a held lock was reported free")
	}
	// Reading it twice is still reading: the holder keeps it.
	if again, err := lockHeld(path); err != nil || !again {
		t.Fatal("reading the lock disturbed it", again, err)
	}
	lock.Close()
	if held, err = lockHeld(path); err != nil || held {
		t.Fatal("a released lock was reported held", held, err)
	}
}
