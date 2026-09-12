//go:build unix

package tui

import (
	"bytes"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// Translate only modified Enter before Bubble Tea v1 parses input. Its unknown
// CSI messages borrow a reused buffer and cannot safely be inspected in Update.
// Embedding the file preserves raw mode, cancellation and terminal detection.
type keyboardReader struct {
	*os.File
	paste bool
}

func keyboardInput(file *os.File) io.Reader { return &keyboardReader{File: file} }

var keyboardSequences = [][]byte{
	[]byte("\x1b[13;5u"), []byte("\x1b[27;5;13~"),
	[]byte("\x1b[200~"), []byte("\x1b[201~"),
}

func (r *keyboardReader) translate(data []byte, final bool) (out, pending []byte) {
	for len(data) > 0 {
		size := 1
		if data[0] == '\x1b' {
			if len(data) == 1 && !final {
				return out, data
			}
			if len(data) > 1 && data[1] == '[' {
				size = 2
				for size < len(data) && (data[size] < 0x40 || data[size] > 0x7e) {
					size++
				}
				if size == len(data) && !final && len(data) < 48 {
					return out, data
				}
				if size < len(data) {
					size++
				}
			} else if len(data) > 1 && data[1] == 'O' {
				if len(data) < 3 && !final {
					return out, data
				}
				size = min(3, len(data))
			}
		} else {
			if !utf8.FullRune(data) && !final {
				return out, data
			}
			_, size = utf8.DecodeRune(data)
		}
		seq := data[:size]
		if !r.paste && (bytes.Equal(seq, keyboardSequences[0]) || bytes.Equal(seq, keyboardSequences[1])) {
			out = append(out, '\x13')
		} else {
			out = append(out, seq...)
		}
		if bytes.Equal(seq, keyboardSequences[2]) {
			r.paste = true
		}
		if bytes.Equal(seq, keyboardSequences[3]) {
			r.paste = false
		}
		data = data[size:]
	}
	return out, nil
}

func (r *keyboardReader) Read(p []byte) (int, error) {
	if len(p) < 64 {
		return r.File.Read(p)
	} // Bubble Tea reads 256-byte chunks.
	// Reserve enough output capacity to finish a sequence split at a read edge.
	raw := make([]byte, len(p)-48)
	n, err := r.File.Read(raw)
	if n == 0 {
		return 0, err
	}
	out, pending := r.translate(raw[:n], false)
	deadline := time.Now().Add(25 * time.Millisecond)
	for len(pending) > 0 && len(out)+len(pending) < len(p) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		fds := []unix.PollFd{{Fd: int32(r.Fd()), Events: unix.POLLIN}}
		ready, pollErr := unix.Poll(fds, int(remaining.Milliseconds())+1)
		if pollErr == unix.EINTR {
			continue
		}
		if pollErr != nil || ready == 0 || fds[0].Revents&unix.POLLIN == 0 {
			break
		}
		var next [1]byte
		count, readErr := r.File.Read(next[:])
		if count == 0 || readErr != nil {
			break
		}
		data := append(append([]byte{}, pending...), next[0])
		var extra []byte
		extra, pending = r.translate(data, false)
		out = append(out, extra...)
	}
	if len(pending) > 0 {
		extra, _ := r.translate(pending, true)
		out = append(out, extra...)
	}
	return copy(p, out), err
}
