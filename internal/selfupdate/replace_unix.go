//go:build !windows

package selfupdate

import (
	"os"
	"path/filepath"
)

func (r *Replacement) replace() error {
	backup, err := os.CreateTemp(filepath.Dir(r.Target), ".cxz-previous-*")
	if err != nil {
		return err
	}
	name := backup.Name()
	defer os.Remove(name)
	if err := backup.Close(); err != nil {
		return err
	}
	if err := copyExecutable(r.Target, name, r.original.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(name, r.Previous); err != nil {
		return err
	}
	// Running processes retain the old inode while new invocations see the new one.
	return os.Rename(r.Candidate, r.Target)
}
