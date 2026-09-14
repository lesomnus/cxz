// Package wisp implements the private, stdio workspace-helper protocol.
// It runs as the container's remote user and never reads agent credentials.
package wisp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Version = 1

type Entry struct {
	Name       string
	Directory  bool
	Executable bool
	LinkTarget string
	Symlink    bool
}
type Request struct {
	Path      string
	Operation string
	Session   string
	Secret    []byte
}
type Response struct {
	Version    int
	Entries    []Entry
	Done       bool
	Truncated  bool
	Error      string
	Secrets    bool
	SecretPath string
}

func Serve(in io.Reader, out io.Writer) error {
	enc := json.NewEncoder(out)
	store := &secretStore{}
	defer store.clear("")
	if err := enc.Encode(Response{Version: Version, Secrets: true}); err != nil {
		return err
	}
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 4096), 256*1024)
	for scan.Scan() {
		var req Request
		if err := json.Unmarshal(scan.Bytes(), &req); err != nil {
			clear(scan.Bytes())
			clear(req.Secret)
			return fmt.Errorf("invalid wisp request")
		}
		clear(scan.Bytes())
		if req.Operation != "" {
			response := store.handle(req)
			clear(req.Secret)
			if err := enc.Encode(response); err != nil {
				return err
			}
			continue
		}
		clear(req.Secret)
		if err := list(req.Path, enc.Encode); err != nil {
			return err
		}
	}
	return scan.Err()
}

func printable(s string) bool {
	return utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}

func list(path string, send func(any) error) error {
	fail := func() error { return send(Response{Done: true, Error: "directory unavailable or permission denied"}) }
	if len(path) > 4096 || strings.ContainsRune(path, 0) {
		return fail()
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		u, err := user.Current()
		if err != nil || u.HomeDir == "" {
			return fail()
		}
		path = u.HomeDir + strings.TrimPrefix(path, "~")
	}
	if !filepath.IsAbs(path) {
		return fail()
	}
	f, err := os.Open(path)
	if err != nil {
		return fail()
	}
	defer f.Close()
	count := 0
	for {
		// Read names incrementally, without shell glob expansion or subprocesses.
		batch, err := f.ReadDir(16)
		if err != nil && err != io.EOF {
			return fail()
		}
		for _, d := range batch {
			if count == 2048 {
				return send(Response{Done: true, Truncated: true})
			}
			count++
			if !printable(d.Name()) {
				continue
			}
			info, e := d.Info()
			if e != nil {
				continue
			}
			entry := Entry{Name: d.Name(), Directory: d.IsDir(), Executable: info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0, Symlink: info.Mode()&os.ModeSymlink != 0}
			if entry.Symlink {
				full := filepath.Join(path, d.Name())
				entry.LinkTarget, _ = os.Readlink(full)
				if !printable(entry.LinkTarget) {
					entry.LinkTarget = "(unprintable target)"
				}
				if target, e := os.Stat(full); e == nil {
					entry.Directory = target.IsDir()
				}
			}
			if e := send(Response{Entries: []Entry{entry}}); e != nil {
				return e
			}
		}
		if err == io.EOF {
			return send(Response{Done: true})
		}
	}
}
