package memorymigration

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// AsOwner is called only by the disposable retained-volume helper process.
func AsOwner(root string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	path := filepath.Join(root, "resources.db")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		info, err = os.Lstat(root)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked resource storage")
	}
	owner := info.Sys().(*syscall.Stat_t)
	if err = syscall.Setgroups(nil); err != nil {
		return err
	}
	if err = syscall.Setgid(int(owner.Gid)); err != nil {
		return err
	}
	return syscall.Setuid(int(owner.Uid))
}
