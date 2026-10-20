package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// A forced check used to race the rollout tick's own save of the same file and
// lose: the release it had just discovered was gone, and the installation went
// on offering the old one with no sign that anything had happened. Every other
// writer of cxz-update.json holds the rollout lock; this is the one that did
// not. See cxz#247.
func TestAForcedCheckWaitsForTheRolloutLockInsteadOfRacingIt(t *testing.T) {
	root := t.TempDir()
	defer func(d time.Duration) { rolloutWait = d }(rolloutWait)
	rolloutWait = 300 * time.Millisecond
	ctx := context.Background()

	rollout, err := core.Lock(filepath.Join(root, "rollout.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = holdRollout(ctx, root); err == nil {
		t.Fatal("a check went ahead while a rollout held the lock")
	}
	if err = rollout.Close(); err != nil {
		t.Fatal(err)
	}

	release, err := holdRollout(ctx, root)
	if err != nil {
		t.Fatal("a free lock was not taken:", err)
	}
	release()
	// Released rather than merely dropped, or the next tick would find the lock
	// held by a command that has already finished.
	again, err := core.Lock(filepath.Join(root, "rollout.lock"))
	if err != nil {
		t.Fatal("the lock outlived the command that took it:", err)
	}
	again.Close()
}
