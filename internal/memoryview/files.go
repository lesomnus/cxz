//go:build linux

// Package memoryview reads retained agent instructions, memory and native history.
package memoryview

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/lesomnus/cxz/internal/accounts"
)

func allowed(agent, name string) bool {
	names := []string{"skills", "rules", "memories", "memory", "history.jsonl"}
	if agent == "claude" {
		names = append(names, "CLAUDE.md", "projects", "plans", "agents", "commands", "todos")
	}
	if agent == "codex" {
		names = append(names, "AGENTS.md", "sessions", "archived_sessions")
	}
	for _, v := range names {
		if v == name {
			return true
		}
	}
	return false
}
func hidden(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, ".cxz-memory-") || n == "auth.json" || n == ".credentials.json" || n == ".claude.json" || n == ".env" || strings.HasPrefix(n, ".env.")
}

// openBelow opens each component without following symlinks, including during
// concurrent agent writes. It never creates directories or changes permissions.
func openBelow(root, rel string) (*os.File, error) {
	f, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	if rel == "" {
		return f, nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
		if i < len(parts)-1 {
			flags |= syscall.O_DIRECTORY
		}
		fd, e := syscall.Openat(int(f.Fd()), part, flags, 0)
		f.Close()
		if e != nil {
			return nil, e
		}
		f = os.NewFile(uintptr(fd), part)
	}
	return f, nil
}
func Read(ctx context.Context, root string, q Query) (Page, error) {
	var out Page
	s := q.Session
	if s.CreateID == "" {
		return out, fmt.Errorf("session has no retained profile identity")
	}
	if err := accounts.Validate(s.Account, s.Kind); err != nil {
		return out, err
	}
	rel := q.Path
	if rel == "." {
		rel = ""
	}
	if err := validatePath(s.Kind, rel); err != nil {
		return out, err
	}
	base := accounts.Config(accounts.SessionRoot(root, s.CreateID), s.Account)
	out.Path = rel
	out.Location = filepath.Join(base, rel)
	// Walk the profile path beneath the state root too, so symlinked profiles
	// cannot redirect the reader to credentials or another session.
	profile, _ := filepath.Rel(root, out.Location)
	f, err := openBelow(root, profile)
	if os.IsNotExist(err) && rel == "" {
		out.Directory = true
		out.Note = "No retained agent profile found."
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return out, err
	}
	out.Directory = st.IsDir()
	if out.Directory {
		entries, err := f.ReadDir(EntryLimit + 1)
		if err != nil && err != io.EOF {
			return out, err
		}
		if len(entries) > EntryLimit {
			out.Truncated = true
			entries = entries[:EntryLimit]
			out.Note = "Directory listing limited to 1000 entries."
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			if hidden(entry.Name()) || entry.Type()&os.ModeSymlink != 0 || (rel == "" && !allowed(s.Kind, entry.Name())) {
				continue
			}
			fd, err := syscall.Openat(int(f.Fd()), entry.Name(), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
			if err != nil {
				continue
			}
			child := os.NewFile(uintptr(fd), entry.Name())
			info, err := child.Stat()
			child.Close()
			if err != nil {
				continue
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				continue
			}
			out.Entries = append(out.Entries, Entry{Name: entry.Name(), Directory: entry.IsDir(), Size: info.Size()})
		}
		sort.Slice(out.Entries, func(i, j int) bool {
			a, b := out.Entries[i], out.Entries[j]
			if a.Directory != b.Directory {
				return a.Directory
			}
			return a.Name < b.Name
		})
		if len(out.Entries) == 0 && out.Note == "" {
			out.Note = "No browsable memory, instructions or history files here."
		}
		return out, nil
	}
	if !st.Mode().IsRegular() {
		return out, fmt.Errorf("only regular text files can be previewed")
	}
	b, err := io.ReadAll(io.LimitReader(f, FileLimit+1))
	if err != nil {
		return out, err
	}
	if len(b) > FileLimit {
		out.Truncated = true
		b = b[:FileLimit]
		out.Note = "Preview limited to the first 256 KiB."
	}
	if strings.IndexByte(string(b), 0) >= 0 {
		return out, fmt.Errorf("binary file cannot be previewed")
	}
	if out.Truncated {
		for len(b) > 0 && !utf8.Valid(b) && len(b) > FileLimit-4 {
			b = b[:len(b)-1]
		}
	}
	if !utf8.Valid(b) {
		return out, fmt.Errorf("file is not UTF-8 text")
	}
	out.Content = string(b)
	return out, nil
}
func Serve(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	var q Query
	if err := json.NewDecoder(io.LimitReader(in, 65536)).Decode(&q); err != nil {
		return err
	}
	p, err := Read(ctx, root, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(p)
}

func validatePath(agent, rel string) error {
	if filepath.IsAbs(rel) || strings.ContainsAny(rel, "\\\x00") {
		return fmt.Errorf("invalid memory path")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || part == "." || (part == "" && rel != "") || hidden(part) {
			return fmt.Errorf("path is outside browsable agent data")
		}
	}
	if rel != "" && !allowed(agent, strings.Split(rel, "/")[0]) {
		return fmt.Errorf("path is outside browsable agent data")
	}

	return nil
}
