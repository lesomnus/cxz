package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
)

// Install copies a local executable into its permanent directory. Existing
// installations use the same replacement lock and backup as self-update.
func Install(source, target string) (bool, error) {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false, err
	}
	if !sourceInfo.Mode().IsRegular() {
		return false, fmt.Errorf("source is not a regular executable: %s", source)
	}
	targetInfo, err := os.Lstat(target)
	if err == nil {
		if !targetInfo.Mode().IsRegular() {
			return false, fmt.Errorf("installation target is not a regular file: %s", target)
		}
		if os.SameFile(sourceInfo, targetInfo) {
			return false, nil
		}
		r, err := Prepare(target)
		if err != nil {
			return false, err
		}
		defer r.Close()
		if err := r.Stage(source); err != nil {
			return false, err
		}
		return r.Apply()
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, err
	}
	lock, err := core.Lock(filepath.Join(dir, "."+filepath.Base(target)+".update.lock"))
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		if err == nil {
			err = fmt.Errorf("executable was installed concurrently; retry self-install")
		}
		return false, err
	}
	f, err := os.CreateTemp(dir, ".cxz-install-*.exe")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	if err := f.Close(); err != nil {
		return false, err
	}
	if err := copyExecutable(source, f.Name(), sourceInfo.Mode().Perm()); err != nil {
		return false, err
	}
	if err := os.Rename(f.Name(), target); err != nil {
		return false, err
	}
	return true, core.SyncDir(dir)
}
