// Package filemap snapshots host files and applies them before agent startup.
package filemap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
)

const Limit = 2 * 1024 * 1024
const maxFiles = 512
const filename = "file-mappings.json"

type Mapping struct {
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Agent string `json:"agent,omitempty"`
}
type File struct {
	Dst        string `json:"dst"`
	Agent      string `json:"agent,omitempty"`
	Content    []byte `json:"content"`
	Executable bool   `json:"executable,omitempty"`
}
type Bundle struct {
	Files []File `json:"files"`
}

// ValidateMappingDestination also permits a whole directory at a session root.
func ValidateMappingDestination(dst, agent string) error {
	return validateDestination(dst, agent, true)
}
func ValidateDestination(dst, agent string) error {
	return validateDestination(dst, agent, false)
}
func validateDestination(dst, agent string, directory bool) error {
	if agent != "" && agent != "claude" && agent != "codex" {
		return fmt.Errorf("agent must be claude or codex")
	}
	if len(dst) > 4096 {
		return fmt.Errorf("destination too long")
	}
	if strings.ContainsAny(dst, "\x00\r\n") || strings.Contains(dst, "\\") {
		return fmt.Errorf("invalid destination")
	}
	for _, part := range strings.Split(dst, "/") {
		if part == ".." {
			return fmt.Errorf("destination must not contain ..")
		}
	}
	rest := dst
	for _, v := range []string{"${AGENT_CONFIG_DIR}", "${SESSION_HOME}", "${WORKSPACE}"} {
		if directory && dst == v {
			return nil
		}
		if strings.HasPrefix(dst, v+"/") {
			rest = strings.TrimPrefix(dst, v)
			break
		}
	}
	if !path.IsAbs(rest) || strings.Contains(rest, "$") || path.Clean(rest) == "/" {
		return fmt.Errorf("dst must be an absolute file path or start with ${AGENT_CONFIG_DIR}/, ${SESSION_HOME}/, or ${WORKSPACE}/")
	}
	return nil
}

func Snapshot(mappings []Mapping) (Bundle, error) {
	var bundle Bundle
	total := 0
	for _, m := range mappings {
		if err := ValidateMappingDestination(m.Dst, m.Agent); err != nil {
			return bundle, err
		}
		src := m.Src
		if strings.HasPrefix(src, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return bundle, err
			}
			src = filepath.Join(home, src[2:])
		}
		src, err := filepath.Abs(src)
		if err != nil {
			return bundle, err
		}
		err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("source must contain regular files only: %s", path)
			}
			if len(bundle.Files) >= maxFiles {
				return fmt.Errorf("file mappings exceed %d files", maxFiles)
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			st, err := f.Stat()
			if err != nil {
				return err
			}
			if !st.Mode().IsRegular() {
				return fmt.Errorf("not a regular file: %s", path)
			}
			b, err := io.ReadAll(io.LimitReader(f, int64(Limit-total+1)))
			if err != nil {
				return err
			}
			total += len(b)
			if total > Limit {
				return fmt.Errorf("file mappings exceed 2 MiB")
			}
			dst := m.Dst
			if path != src {
				rel, _ := filepath.Rel(src, path)
				dst = strings.TrimSuffix(dst, "/") + "/" + filepath.ToSlash(rel)
			}
			bundle.Files = append(bundle.Files, File{Dst: dst, Agent: m.Agent, Content: b, Executable: st.Mode().Perm()&0111 != 0})
			return nil
		})
		if err != nil {
			return Bundle{}, fmt.Errorf("source %s: %w", m.Src, err)
		}
	}
	return bundle, bundle.Validate()
}
func (b Bundle) Validate() error {
	total := 0
	if len(b.Files) > maxFiles {
		return fmt.Errorf("too many mapped files")
	}
	seen := map[string][]string{}
	for _, f := range b.Files {
		if err := ValidateDestination(f.Dst, f.Agent); err != nil {
			return err
		}
		key := filepath.Clean(f.Dst)
		for _, a := range seen[key] {
			if a == "" || f.Agent == "" || a == f.Agent {
				return fmt.Errorf("duplicate destination: %s", f.Dst)
			}
		}
		seen[key] = append(seen[key], f.Agent)
		total += len(f.Content)
		if total > Limit {
			return fmt.Errorf("file mappings exceed 2 MiB")
		}
	}
	return nil
}
func Decode(data []byte) (Bundle, error) {
	var b Bundle
	if len(data) > 3*Limit {
		return b, fmt.Errorf("file mapping payload too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&b); err != nil {
		return b, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return b, fmt.Errorf("expected one file mapping bundle")
	}
	return b, b.Validate()
}
func Load(root string) (Bundle, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, filename))
	if os.IsNotExist(err) {
		return Bundle{}, false, nil
	}
	if err != nil {
		return Bundle{}, false, err
	}
	b, err := Decode(data)
	return b, true, err
}
func Save(root string, b Bundle) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return core.WriteJSON(filepath.Join(root, filename), b)
}

// Apply refreshes configured files. Unmapped files are retained, not deleted.
func Apply(root, agent, config, home, workspace string) error {
	b, _, err := Load(root)
	if err != nil {
		return err
	}
	replace := strings.NewReplacer("${AGENT_CONFIG_DIR}", config, "${SESSION_HOME}", home, "${WORKSPACE}", workspace)
	seen := map[string]bool{}
	var resolved []File
	for _, f := range b.Files {
		if f.Agent != "" && f.Agent != agent {
			continue
		}
		dst := filepath.Clean(replace.Replace(f.Dst))
		if seen[dst] {
			return fmt.Errorf("duplicate resolved destination: %s", dst)
		}
		seen[dst] = true
		f.Dst = dst
		resolved = append(resolved, f)
	}
	for _, f := range resolved {
		if err := write(f.Dst, f); err != nil {
			return fmt.Errorf("copy mapped file to %s: %w", f.Dst, err)
		}
	}
	return nil
}
func write(dst string, f File) error {
	// Reject symlink parents: copies must land at their declared destination.
	dir := filepath.Dir(dst)
	for p := dir; ; p = filepath.Dir(p) {
		st, err := os.Lstat(p)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination parent is a symlink")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if st, err := os.Lstat(dst); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("destination is not a regular file")
	}
	mode := os.FileMode(0600)
	if f.Executable {
		mode = 0700
	}
	tmp, err := os.CreateTemp(dir, ".cxz-map-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(f.Content)
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), dst)
}
