package auxiliary

import (
	"testing"
	"time"
)

// A watcher is woken where the state is written, which is what lets a client be
// told instead of asking.
func TestWatchIsWokenByAWrite(t *testing.T) {
	c, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	wake, stop := c.Watch("a")
	defer stop()
	select {
	case <-wake:
		t.Fatal("woken before anything was written")
	default:
	}
	c.mu.Lock()
	err = c.save("a", State{Seen: 1})
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("a write did not wake the watcher")
	}
	// A wake already pending covers the next change too, so a write never
	// blocks on a watcher that has not read yet.
	for i := 0; i < 4; i++ {
		c.mu.Lock()
		err = c.save("a", State{Seen: uint64(i + 2)})
		c.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-wake:
	default:
		t.Fatal("no wake pending after four writes")
	}
	// Another session's write is not this watcher's news.
	c.mu.Lock()
	err = c.save("b", State{Seen: 1})
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
		t.Fatal("woken for another session")
	default:
	}
	// Stopping leaves nothing behind to wake.
	stop()
	c.mu.Lock()
	watching := len(c.watchers)
	c.mu.Unlock()
	if watching != 0 {
		t.Fatal("a stopped watcher was kept", watching)
	}
}
