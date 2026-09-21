package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
)

type Replacement struct {
	Target, Previous, Candidate string
	lock                        *os.File
	original                    os.FileInfo
}

// Prepare checks directory permissions and locks the resolved executable before
// an expensive build. A link in PATH keeps pointing at its original target.
func Prepare(target string) (*Replacement, error) {
	path, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("executable is not a regular file: %s", path)
	}
	lock, err := core.Lock(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".update.lock"))
	if err != nil {
		return nil, fmt.Errorf("lock executable for update (write access to %s is required): %w", filepath.Dir(path), err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cxz-update-*.exe")
	if err != nil {
		lock.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		lock.Close()
		return nil, err
	}
	previous := path + ".previous"
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		previous = strings.TrimSuffix(path, filepath.Ext(path)) + ".previous.exe"
	}
	return &Replacement{Target: path, Previous: previous, Candidate: f.Name(), lock: lock, original: st}, nil
}

func (r *Replacement) Close() {
	os.Remove(r.Candidate)
	r.lock.Close()
}

func (r *Replacement) Stage(source string) error {
	return copyExecutable(source, r.Candidate, r.original.Mode().Perm())
}

// Apply returns changed=true even if a later directory sync fails. Callers must
// accurately report a replaced CLI separately from a failed manager refresh.
func (r *Replacement) Apply() (changed bool, err error) {
	current, err := os.Stat(r.Target)
	if err != nil {
		return false, err
	}
	if !os.SameFile(current, r.original) || current.Size() != r.original.Size() || !current.ModTime().Equal(r.original.ModTime()) {
		return false, fmt.Errorf("executable changed during the build; retry self-update")
	}
	if err := r.replace(); err != nil {
		return false, err
	}
	return true, core.SyncDir(filepath.Dir(r.Target))
}

func copyExecutable(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Chmod(mode); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
