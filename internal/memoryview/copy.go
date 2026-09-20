//go:build linux

package memoryview

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"golang.org/x/sys/unix"
)

type copyEntry struct {
	path, content string
	directory     bool
}

const CopyLimit = 16 * 1024 * 1024

func Copy(ctx context.Context, sourceRoot, targetRoot string, q CopyQuery) (string, error) {
	if q.Source.Kind != q.Target.Kind {
		return "", fmt.Errorf("source and target must use the same agent")
	}
	if q.Source.ID == q.Target.ID {
		return "", fmt.Errorf("choose a different target session")
	}
	if q.Target.CreateID == "" || !regexp.MustCompile("^[a-f0-9]{24}$").MatchString(q.Target.ID) {
		return "", fmt.Errorf("invalid target session identity")
	}
	if err := accounts.Validate(q.Target.Account, q.Target.Kind); err != nil {
		return "", err
	}
	if q.Path == "" || q.TargetPath == "" {
		return "", fmt.Errorf("select a file or folder, not the whole profile")
	}
	if err := validatePath(q.Target.Kind, q.TargetPath); err != nil {
		return "", err
	}
	var entries []copyEntry
	size := 0
	var visit func(string, string) error
	visit = func(name, relative string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(entries) >= EntryLimit {
			return fmt.Errorf("copy exceeds 1000 entries")
		}
		p, err := Read(ctx, sourceRoot, Query{Session: q.Source, Path: name})
		if err != nil {
			return err
		}
		if p.Truncated {
			return fmt.Errorf("copy source exceeds preview limit: %s", name)
		}
		size += len(p.Content)
		if size > CopyLimit {
			return fmt.Errorf("copy exceeds 16 MiB")
		}
		entries = append(entries, copyEntry{relative, p.Content, p.Directory})
		for _, child := range p.Entries {
			if err := visit(path.Join(name, child.Name), path.Join(relative, child.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(q.Path, ""); err != nil {
		return "", err
	}
	config := accounts.Config(accounts.SessionRoot(targetRoot, q.Target.CreateID), q.Target.Account)
	configRel, _ := filepath.Rel(targetRoot, config)
	dir, err := openBelow(targetRoot, configRel)
	if err != nil {
		return "", fmt.Errorf("target profile unavailable; initialize the target session first: %w", err)
	}
	defer dir.Close()
	stat, err := dir.Stat()
	if err != nil {
		return "", err
	}
	owner := stat.Sys().(*syscall.Stat_t)
	// These are the same leases used by supervisor startup and account login.
	for _, rel := range []string{filepath.Join("sessions", q.Target.ID, "supervisor.lock"), filepath.Join(filepath.Dir(configRel), "login.lock")} {
		lock, err := openBelow(targetRoot, rel)
		if err != nil {
			return "", fmt.Errorf("target session lock unavailable: %w", err)
		}
		defer lock.Close()
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			return "", fmt.Errorf("stop the target agent and finish login before copying")
		}
	}
	parent := path.Dir(q.TargetPath)
	if parent != "." {
		for _, part := range strings.Split(parent, "/") {
			fd, e := unix.Openat(int(dir.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if e == syscall.ENOENT {
				if e = unix.Mkdirat(int(dir.Fd()), part, 0700); e != nil {
					return "", e
				}
				fd, e = unix.Openat(int(dir.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if e == nil && os.Geteuid() == 0 {
					e = unix.Fchown(fd, int(owner.Uid), int(owner.Gid))
				}
			}
			if e != nil {
				if fd >= 0 {
					unix.Close(fd)
				}
				return "", e
			}
			next := os.NewFile(uintptr(fd), part)
			defer next.Close()
			dir = next
		}
	}
	name := path.Base(q.TargetPath)
	stage := ".cxz-memory-" + core.ID()
	if err := unix.Mkdirat(int(dir.Fd()), stage, 0700); err != nil {
		return "", err
	}
	// Stage is private and addressed via the open parent descriptor, not a
	// re-resolved pathname. Publish with NOREPLACE, including empty directories.
	stagePath := fmt.Sprintf("/proc/self/fd/%d/%s", dir.Fd(), stage)
	defer os.RemoveAll(stagePath)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		target := filepath.Join(stagePath, "payload", entry.path)
		if entry.directory {
			err = os.MkdirAll(target, 0700)
		} else {
			err = os.MkdirAll(filepath.Dir(target), 0700)
			if err == nil {
				err = os.WriteFile(target, []byte(entry.content), 0600)
			}
		}
		if err != nil {
			return "", err
		}
		if os.Geteuid() == 0 {
			if err = os.Chown(target, int(owner.Uid), int(owner.Gid)); err != nil {
				return "", err
			}
		}
	}
	if err = unix.Renameat2(int(dir.Fd()), stage+"/payload", int(dir.Fd()), name, unix.RENAME_NOREPLACE); err != nil {
		if err == syscall.EEXIST {
			return "", fmt.Errorf("destination already exists; choose a different path")
		}
		return "", err
	}
	return fmt.Sprintf("Copied %d entries (%d bytes) to %s", len(entries), size, q.TargetPath), nil
}
func ServeCopy(ctx context.Context, sourceRoot, targetRoot string, in io.Reader, out io.Writer) error {
	var q CopyQuery
	if err := json.NewDecoder(io.LimitReader(in, 65536)).Decode(&q); err != nil {
		return err
	}
	message, err := Copy(ctx, sourceRoot, targetRoot, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(message)
}
