//go:build unix

package tui

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const extendedKeyboard = true

// Translate Kitty disambiguated keys before Bubble Tea v1 parses input. Its unknown
// CSI messages borrow a reused buffer and cannot safely be inspected in Update.
// Embedding the file preserves raw mode, cancellation and terminal detection.
type keyboardReader struct {
	*os.File
	paste          bool
	kittyConfirmed bool
	kittyFlags     int
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
		if translated, ok := r.kittyKey(seq); !r.paste && ok {
			out = append(out, translated...)
		} else if !r.paste && (bytes.Equal(seq, keyboardSequences[0]) || bytes.Equal(seq, keyboardSequences[1])) {
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

// Bridge the disambiguation subset to Bubble Tea v1's legacy key decoder.
// Do not request event/alternate-key/all-text flags that v1 cannot decode.
func (r *keyboardReader) kittyKey(seq []byte) ([]byte, bool) {
	if r.paste || !bytes.HasPrefix(seq, []byte("\x1b[")) || len(seq) < 4 || seq[len(seq)-1] != 'u' {
		return nil, false
	}
	body := string(seq[2 : len(seq)-1])
	if strings.HasPrefix(body, "?") {
		flags, err := strconv.Atoi(body[1:])
		if err != nil || flags < 0 {
			return nil, false
		}
		r.kittyConfirmed, r.kittyFlags = true, flags
		return nil, true
	}
	fields := strings.Split(body, ";")
	if len(fields) > 2 {
		return nil, false
	}
	key, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil, false
	}
	mod := 1
	if len(fields) == 2 {
		mod, err = strconv.Atoi(fields[1])
		if err != nil || mod < 1 {
			return nil, false
		}
	}
	mod = (mod - 1) &^ (64 | 128) // Caps/Num Lock do not change shortcuts.
	if key >= 57399 && key <= 57416 {
		key = int("0123456789./*-+\r=,"[key-57399])
	}
	if key >= 57417 && key <= 57426 && mod&^7 == 0 {
		keys := []string{"1D", "1C", "1A", "1B", "5~", "6~", "1H", "1F", "2~", "3~"}
		legacy := keys[key-57417]
		if mod == 0 {
			if legacy[0] == '1' {
				legacy = legacy[1:]
			}
			return []byte("\x1b[" + legacy), true
		}
		return []byte("\x1b[" + legacy[:1] + ";" + strconv.Itoa(mod+1) + legacy[1:]), true
	}
	if key == 13 && mod == 4 {
		return []byte{'\x13'}, true
	}
	if key == 96 && mod == 4 {
		return []byte("\x1b[34~"), true
	} // reserved terminal toggle bridge (F20)
	if key == 9 && mod == 1 {
		return []byte("\x1b[Z"), true
	}
	if key > 127 && key <= utf8.MaxRune && utf8.ValidRune(rune(key)) && (mod == 0 || mod == 1) {
		return []byte(string(rune(key))), true
	}
	if mod&^7 != 0 || key < 0 || key > 127 {
		return nil, false
	}
	if mod&4 != 0 {
		switch {
		case key >= 'a' && key <= 'z':
			key -= 'a' - 1
		case key >= '@' && key <= '_':
			key -= '@'
		case key == ' ':
			key = 0
		case key == '?':
			key = 127
		case key == 127:
			key = 8
		case key == '2':
			key = 0
		case key >= '3' && key <= '7':
			key -= '3' - 27
		case key == '8':
			key = 127
		case key == '/':
			key = 31
		case key == '~':
			key = 30
		default:
			// Remaining ASCII keys keep their legacy value under Ctrl.
		}
	} else if mod&1 != 0 && key >= 'a' && key <= 'z' {
		key -= 'a' - 'A'
	}
	out := []byte{byte(key)}
	if mod&2 != 0 {
		out = append([]byte{'\x1b'}, out...)
	}
	return out, true
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
