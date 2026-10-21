package workspace

import (
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/cxzupdate"
)

// A lock collision is a footnote on a state that did not change. Writing it as
// the state is how an installation that had finished rolling out came to report
// "project operation in progress" for twelve hours while nothing was in
// progress. See cxz#257.
func TestALockCollisionDoesNotBecomeTheState(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	if err := cxzupdate.Save(m.Root, cxzupdate.State{State: "healthy"}); err != nil {
		t.Fatal(err)
	}
	m.footnoteUpdate("a rollout holds the lock: already running")
	s, err := cxzupdate.Load(m.Root)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != "healthy" {
		t.Fatal("the installation's state was rewritten by a lock collision:", s.State)
	}
	if s.Reason == "" {
		t.Fatal("the collision was not recorded at all, which is the silence cxz#222 was about")
	}
	// Unchanged twice is written once, or a busy installation rewrites the file
	// every thirty seconds forever.
	info, err := modTime(cxzupdate.StatePath(m.Root))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	m.footnoteUpdate("a rollout holds the lock: already running")
	again, err := modTime(cxzupdate.StatePath(m.Root))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Equal(info) {
		t.Fatal("an unchanged footnote was written again")
	}
}

// Losing the race once is not losing it forever: a client holds a project's
// lock for a moment, and the pass waits that moment out rather than skipping
// the project on every tick.
func TestAPassWaitsOutAClientHoldingAProjectLock(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()
	go func() {
		time.Sleep(120 * time.Millisecond)
		mu.Unlock()
	}()
	start := time.Now()
	if !tryLockFor(&mu, 2*time.Second) {
		t.Fatal("the pass gave up on a lock that was free a moment later")
	}
	if took := time.Since(start); took < 100*time.Millisecond {
		t.Fatal("the lock was taken before it was released, after", took)
	}
	mu.Unlock()

	// A lock that is held for longer than the pass can wait is reported, not
	// waited on forever: the rollout tick has nine other projects to see.
	var held sync.Mutex
	held.Lock()
	defer held.Unlock()
	if tryLockFor(&held, 100*time.Millisecond) {
		t.Fatal("a held lock was reported as taken")
	}
}
