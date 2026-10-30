package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type References struct {
	Paths     map[string]bool
	Revisions map[string]bool
	Versions  map[string]bool
}

func (r References) Protects(path string) bool {
	if r.Revisions[filepath.Base(path)] || r.Versions[filepath.Base(path)] {
		return true
	}
	for ref := range r.Paths {
		if path == ref || strings.HasPrefix(ref, path+string(os.PathSeparator)) || strings.HasPrefix(path, ref+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// ReadReferences only reads small, known operational manifests, never provider
// profiles or conversation bodies. Missing optional files are fine; malformed
// or inaccessible metadata aborts executable GC rather than guessing.
func ReadReferences(ctx context.Context, roots []string, tools string) (References, error) {
	r := References{Paths: map[string]bool{}, Revisions: map[string]bool{}, Versions: map[string]bool{}}
	var visit func(any, string, string)
	visit = func(value any, key, root string) {
		switch v := value.(type) {
		case map[string]any:
			for k, x := range v {
				if strings.EqualFold(k, "ancestors") {
					continue
				}
				visit(x, strings.ToLower(k), root)
			}
		case []any:
			for _, x := range v {
				visit(x, key, root)
			}
		case string:
			if filepath.IsAbs(v) {
				path := v
				if strings.HasPrefix(v, "/cxz/state/data/") {
					path = filepath.Join(root, strings.TrimPrefix(v, "/cxz/state/data/"))
				}
				if strings.HasPrefix(v, "/cxz/tools/") {
					path = filepath.Join(tools, strings.TrimPrefix(v, "/cxz/tools/"))
				}
				r.Paths[filepath.Clean(path)] = true
			}
			if key == "revision" {
				r.Revisions[v] = true
			}
			if key == "version" {
				r.Versions[v] = true
			}
		}
	}
	read := func(path, root string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
		if err != nil {
			return err
		}
		if len(b) > 2<<20 {
			return fmt.Errorf("executable reference metadata too large: %s", path)
		}
		var value any
		if err = json.Unmarshal(b, &value); err != nil {
			return fmt.Errorf("invalid executable reference metadata %s: %w", path, err)
		}
		object, ok := value.(map[string]any)
		if !ok || len(object) == 0 {
			return fmt.Errorf("incomplete executable reference metadata %s", path)
		}
		if filepath.Base(path) == "session.json" {
			id, _ := object["id"].(string)
			if id == "" {
				return fmt.Errorf("session identity missing in executable reference metadata %s", path)
			}
		}
		visit(value, "", root)
		return nil
	}
	for _, root := range roots {
		for _, name := range []string{"runtime-update.json", "manager-update.json", "cxz-update.json", "version-pin.json", "run/maintenance-binary.json", "../runtime.json"} {
			if err := read(filepath.Join(root, name), root); err != nil {
				return r, err
			}
		}
		for _, pattern := range []string{"sessions/*/session.json", "sessions/*/*update*.json"} {
			paths, err := filepath.Glob(filepath.Join(root, pattern))
			if err != nil {
				return r, err
			}
			for _, path := range paths {
				if err = read(path, root); err != nil {
					return r, err
				}
			}
		}
	}
	if tools != "" {
		var selected map[string]string
		if b, err := os.ReadFile(filepath.Join(tools, "releases.json")); err == nil {
			if err = json.Unmarshal(b, &selected); err != nil {
				return r, err
			}
			for kind, version := range selected {
				if kind == "claude" || kind == "codex" || kind == "gh" {
					r.Paths[filepath.Join(tools, kind, version)] = true
				}
			}
		} else if !os.IsNotExist(err) {
			return r, err
		}
		if target, err := filepath.EvalSymlinks(filepath.Join(tools, "cxz")); err == nil {
			r.Paths[target] = true
		}
	}
	if exe, err := os.Executable(); err == nil {
		r.Paths[strings.TrimSuffix(exe, " (deleted)")] = true
	}
	return r, nil
}

var versionName = regexp.MustCompile(`^(?:[a-f0-9]{40}|[0-9]+\.[0-9]+\.[0-9]+|v[0-9]+\.[0-9]+\.[0-9]+|source-[a-f0-9]{12,40})$`)

// VersionCandidates holds the publication lock and refreshes the complete
// reference set immediately before each removal. Selected, mapped, session and
// rollback references take precedence over both age and pressure.
func VersionCandidates(ctx context.Context, groups []string, lock string, references func(context.Context) (References, error), p Policy, now time.Time, pressure bool) ([]Candidate, error) {
	refs, err := references(ctx)
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if !PlainPath(group) {
			continue
		}
		es, err := os.ReadDir(group)
		if err != nil {
			return out, err
		}
		type version struct {
			path string
			time time.Time
		}
		var unused []version
		for _, e := range es {
			if !e.IsDir() || !versionName.MatchString(e.Name()) {
				continue
			}
			path := filepath.Join(group, e.Name())
			if refs.Protects(path) {
				continue
			}
			st, err := e.Info()
			if err != nil {
				return out, err
			}
			unused = append(unused, version{path, st.ModTime()})
		}
		sort.Slice(unused, func(i, j int) bool {
			if unused[i].time.Equal(unused[j].time) {
				return unused[i].path < unused[j].path
			}
			return unused[i].time.After(unused[j].time)
		})
		for i, v := range unused {
			if i < p.RecentVersions || !pressure && (p.BinaryDays == 0 || now.Sub(v.time) < time.Duration(p.BinaryDays)*24*time.Hour) {
				continue
			}
			out = append(out, Candidate{Path: v.path, Kind: Binaries, Age: v.time, Remove: func(ctx context.Context) (bool, error) {
				return locked(lock, func() (bool, error) {
					fresh, err := references(ctx)
					if err != nil {
						return false, err
					}
					if fresh.Protects(v.path) || !PlainPath(v.path) {
						return false, nil
					}
					err = RemoveTree(ctx, v.path)
					return err == nil, err
				})
			}})
		}
	}
	return out, nil
}
