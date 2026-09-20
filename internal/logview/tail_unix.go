//go:build !windows

package logview

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Tail never repairs journals or follows an unbounded file into memory.
func Tail(path string, limit int) ([]byte, bool, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("not a regular log file")
	}
	start := max(int64(0), st.Size()-int64(limit))
	b, err := io.ReadAll(io.NewSectionReader(f, start, int64(limit)))
	if start > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b, start > 0, err
}
