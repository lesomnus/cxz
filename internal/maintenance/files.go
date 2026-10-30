package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/gofrs/flock"
)

var profileName = regexp.MustCompile(`^[a-f0-9]{32}$`)
var profileAccount = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// PlainPath rejects symlinks in every component. Cleanup must not follow an
// agent-created cache symlink into credentials, a workspace or another volume.
func PlainPath(path string) bool {
	path = filepath.Clean(path)
	for {
		st, err := os.Lstat(path)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return true
		}
		path = parent
	}
}

func locked(path string, fn func() (bool, error)) (bool, error) {
	if !PlainPath(filepath.Dir(path)) {
		return false, nil
	}
	// A Manager may create this in a remote user's private directory. Readers
	// can take an exclusive flock without O_RDWR; keep the empty lease readable
	// so future runtime maintenance is not blocked by a root-owned lock file.
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil && !os.IsExist(e) {
			return false, e
		}
		if e == nil {
			f.Close()
		}
	}
	return lockedExisting(path, fn)
}

func lockedExisting(path string, fn func() (bool, error)) (bool, error) {
	if !PlainPath(path) {
		return false, nil
	}
	l := flock.New(path, flock.SetFlag(os.O_RDONLY))
	defer l.Close()
	ok, err := l.TryLock()
	if err != nil || !ok {
		return false, err
	}
	return fn()
}

// RemoveTree checks cancellation between entries. os.Root keeps removal inside
// the selected disposable directory even if a symlink appears during a pass.
func RemoveTree(ctx context.Context, path string) error {
	if !PlainPath(path) {
		return nil
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return os.Remove(path)
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer r.Close()
	opened, err := r.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(st, opened) {
		return nil
	}
	var clear func(string) error
	clear = func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := r.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		for {
			es, err := f.ReadDir(64)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			for _, e := range es {
				if err := ctx.Err(); err != nil {
					return err
				}
				rel := filepath.Join(name, e.Name())
				if e.IsDir() {
					if err := clear(rel); err != nil {
						return err
					}
				}
				if err := r.Remove(rel); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
		}
	}
	if err = clear("."); err != nil {
		return err
	}
	return os.Remove(path)
}

// Rotate uses copy-and-truncate because inherited O_APPEND descriptors can
// outlive a Manager/runtime. Renaming the current file would strand its writer
// on an unlinked inode and fail to bound storage. Logs are best effort: writes
// racing the copy/truncate boundary can be lost; journals/audit never use this.
func Rotate(path string, limit int64, files int) (bool, error) {
	if !PlainPath(path) {
		return false, nil
	}
	before, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !os.SameFile(before, st) || !st.Mode().IsRegular() || st.Size() <= limit {
		return false, err
	}
	for i := files - 1; i >= 2; i-- {
		if err = os.Rename(path+"."+strconv.Itoa(i-1), path+"."+strconv.Itoa(i)); err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	if files > 1 {
		// Retain a bounded tail even when upgrading from a very large log.
		tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".rotate-")
		if err != nil {
			return false, err
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, io.NewSectionReader(f, max(0, st.Size()-limit), min(st.Size(), limit)))
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return false, err
		}
		if err = os.Rename(tmp.Name(), path+".1"); err != nil {
			return false, err
		}
	}
	err = f.Truncate(0)
	return err == nil, err
}

func LogCandidates(root string, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
	paths := map[string]int{filepath.Join(root, "runtime.log"): 4}
	for _, pattern := range []string{"sessions/*/supervisor.log", "sessions/*/agent.stderr.log", "sessions/*/mcp-*.stderr.log", "projects/*/provision.log"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		for _, path := range matches {
			paths[path] = 3
		}
	}
	return logCandidates(paths, filepath.Join(root, "runtime.log"), p, now, pressure)
}

func ComponentLogCandidates(path string, p Policy, pressure bool) ([]Candidate, error) {
	return logCandidates(map[string]int{path: 3}, "", p, time.Now(), pressure)
}

func logCandidates(paths map[string]int, runtimePath string, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
	var out []Candidate
	for path, files := range paths {
		if p.TemporaryHours > 0 {
			temps, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".rotate-*"))
			for _, temp := range temps {
				st, err := os.Lstat(temp)
				if err != nil || !st.Mode().IsRegular() || !pressure && now.Sub(st.ModTime()) < time.Duration(p.TemporaryHours)*time.Hour {
					continue
				}
				out = append(out, Candidate{Path: temp, Kind: Temporary, Age: st.ModTime(), Remove: func(context.Context) (bool, error) {
					return locked(path+".rotation.lock", func() (bool, error) {
						if !PlainPath(temp) {
							return false, nil
						}
						err := os.Remove(temp)
						return err == nil, err
					})
				}})
			}
		}
		limit := int64(p.ComponentLogMiB) << 20
		if path == runtimePath {
			limit = int64(p.RuntimeLogMiB) << 20
		}
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		if !pressure && st.Size() > limit {
			out = append(out, Candidate{Path: path, Kind: Logs, Age: st.ModTime(), Remove: func(context.Context) (bool, error) {
				return locked(path+".rotation.lock", func() (bool, error) { return Rotate(path, limit, files) })
			}})
		}
		for i := 1; i < files; i++ {
			archive := fmt.Sprintf("%s.%d", path, i)
			st, err := os.Lstat(archive)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			if pressure {
				out = append(out, Candidate{Path: archive, Kind: Logs, Age: st.ModTime(), Remove: func(context.Context) (bool, error) {
					return locked(path+".rotation.lock", func() (bool, error) {
						if !PlainPath(archive) {
							return false, nil
						}
						err := os.Remove(archive)
						return err == nil, err
					})
				}})
			}
		}
	}
	return out, nil
}

// ProfileCandidates only touches tool-owned disposable paths while holding the
// exact login.lock shared by the supervisor and its process-group guard. An
// idle but still running agent remains protected, including background shells.
func ProfileCandidates(root string, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
	profiles, err := filepath.Glob(filepath.Join(root, "session-profiles", "*", "accounts", "*"))
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, profile := range profiles {
		if !profileName.MatchString(filepath.Base(filepath.Dir(filepath.Dir(profile)))) || !profileAccount.MatchString(filepath.Base(profile)) {
			continue
		}
		if !PlainPath(profile) {
			continue
		}
		lock := filepath.Join(profile, "login.lock")
		_, err = lockedExisting(lock, func() (bool, error) {
			marker := filepath.Join(profile, "maintenance-idle")
			st, err := os.Stat(marker)
			if os.IsNotExist(err) {
				if err = os.WriteFile(marker, nil, 0600); err != nil {
					return false, err
				}
				st, err = os.Stat(marker)
			}
			if err != nil {
				return false, err
			}
			idleSince := st.ModTime()
			add := func(path string, kind Kind, age time.Time) {
				out = append(out, Candidate{Path: path, Kind: kind, Age: age, Remove: func(ctx context.Context) (bool, error) {
					return lockedExisting(lock, func() (bool, error) {
						if !PlainPath(path) {
							return false, nil
						}
						if !pressure {
							marker, err := os.Stat(filepath.Join(profile, "maintenance-idle"))
							if err != nil {
								return false, nil
							}
							st, err := os.Lstat(path)
							if err != nil {
								return false, nil
							}
							age := st.ModTime()
							if marker.ModTime().After(age) {
								age = marker.ModTime()
							}
							if now.Sub(age) < time.Duration(p.TemporaryHours)*time.Hour {
								return false, nil
							}
						}
						err := RemoveTree(ctx, path)
						return err == nil, err
					})
				}})
			}
			if p.TemporaryHours > 0 {
				es, err := os.ReadDir(filepath.Join(profile, "tmp"))
				if err != nil && !os.IsNotExist(err) {
					return false, err
				}
				for _, e := range es {
					st, err := e.Info()
					if err != nil {
						continue
					}
					age := st.ModTime()
					if idleSince.After(age) {
						age = idleSince
					}
					if pressure || now.Sub(age) >= time.Duration(p.TemporaryHours)*time.Hour {
						add(filepath.Join(profile, "tmp", e.Name()), Temporary, age)
					}
				}
			}
			if pressure {
				for _, rel := range []string{"home/.cache/go-build", "home/.cache/pip", "home/.npm/_cacache"} {
					path := filepath.Join(profile, filepath.FromSlash(rel))
					if st, err := os.Lstat(path); err == nil && st.IsDir() {
						add(path, Cache, st.ModTime())
					}
				}
			}
			return false, nil
		})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func Combine(collectors ...Collect) Collect {
	return func(ctx context.Context, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
		var out []Candidate
		var errs []error
		for _, collect := range collectors {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			cs, err := collect(ctx, p, now, pressure)
			out = append(out, cs...)
			if err != nil {
				errs = append(errs, err)
			}
		}
		return out, errors.Join(errs...)
	}
}

func LocalFiles(root string) Collect {
	return Combine(func(_ context.Context, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
		return LogCandidates(root, p, now, pressure)
	}, func(_ context.Context, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
		return ProfileCandidates(root, p, now, pressure)
	})
}

// LegacyJournals shares the same writer lock in a runtime and in its Manager's
// view of a stopped project's state. Verification belongs to the session store.
func LegacyJournals(root string, remove func(context.Context, string, string) (bool, error)) Collect {
	return func(ctx context.Context, _ Policy, _ time.Time, _ bool) ([]Candidate, error) {
		paths, err := filepath.Glob(filepath.Join(root, "sessions", "*", "events.jsonl"))
		if err != nil {
			return nil, err
		}
		var out []Candidate
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || !PlainPath(path) {
				continue
			}
			dir := filepath.Dir(path)
			if !PlainPath(filepath.Join(dir, "supervisor.lock")) {
				continue
			}
			if held, err := lockHeld(filepath.Join(dir, "supervisor.lock")); err != nil || held {
				continue
			}
			out = append(out, Candidate{Path: path, Kind: Residue, Age: st.ModTime(), Remove: func(ctx context.Context) (bool, error) {
				return lockedExisting(filepath.Join(dir, "supervisor.lock"), func() (bool, error) {
					if !PlainPath(path) {
						return false, nil
					}
					return remove(ctx, dir, filepath.Base(dir))
				})
			}})
		}
		return out, nil
	}
}

// StagingCandidates knows the installer's temporary naming scheme. Taking its
// exclusive publication lease proves no new installer is still using the data.
func StagingCandidates(patterns []string, lock string, p Policy, now time.Time, pressure bool) ([]Candidate, error) {
	if p.TemporaryHours <= 0 {
		return nil, nil
	}
	var out []Candidate
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return out, err
		}
		for _, path := range paths {
			st, err := os.Lstat(path)
			if err != nil || st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if !pressure && now.Sub(st.ModTime()) < time.Duration(p.TemporaryHours)*time.Hour {
				continue
			}
			out = append(out, Candidate{Path: path, Kind: Temporary, Age: st.ModTime(), Remove: func(ctx context.Context) (bool, error) {
				return locked(lock, func() (bool, error) {
					if !PlainPath(path) {
						return false, nil
					}
					err := RemoveTree(ctx, path)
					return err == nil, err
				})
			}})
		}
	}
	return out, nil
}
