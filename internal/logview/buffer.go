package logview

import (
	"strings"
	"sync"
)

// Buffer retains a bounded stderr tail across concurrent process writes/reads.
type Buffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= FileLimit {
		b.data = append(b.data[:0], p[n-FileLimit:]...)
		b.truncated = true
	} else {
		if len(b.data)+n > FileLimit {
			b.data = append(b.data[:0], b.data[len(b.data)+n-FileLimit:]...)
			b.truncated = true
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := strings.ToValidUTF8(string(b.data), "�")
	if b.truncated {
		text = "[stderr tail: 64 KiB]\n" + text
	}
	return text
}
