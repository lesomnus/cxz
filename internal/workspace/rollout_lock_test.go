package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
)

// A rollout that cannot take the lock must say so. It used to return in
// silence, so an installation that had stopped updating looked exactly like one
// with nothing to do -- for as long as whoever held the lock held it. See
// cxz#222.
func TestARolloutThatCannotTakeTheLockSaysSo(t *testing.T) {
	root := t.TempDir()
	m := &Manager{Root: root}
	if err := cxzupdate.SetPolicy(root, true); err != nil {
		t.Fatal(err)
	}
	held, err := core.Lock(filepath.Join(root, "rollout.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	m.cxzUpdateStep(context.Background())

	s, err := cxzupdate.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != "holding" {
		t.Fatal("a contended rollout left no trace, state is", s.State)
	}
	if s.Reason == "" {
		t.Fatal("it did not say what it was waiting for")
	}
	// And it does not rewrite that state on every tick while it waits.
	before, err := modTime(cxzupdate.StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	m.cxzUpdateStep(context.Background())
	after, err := modTime(cxzupdate.StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("the state was rewritten while nothing changed")
	}
}

func modTime(path string) (time.Time, error) {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return st.ModTime(), nil
}
