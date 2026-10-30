package maintenance

import (
	"fmt"
	"golang.org/x/sys/unix"
	"path/filepath"
)

func DiskSpace(path string) (Space, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return Space{}, err
	}
	return Space{Device: fmt.Sprint(s.Fsid), Total: s.Blocks * uint64(s.Bsize), Available: s.Bavail * uint64(s.Bsize)}, nil
}

func readOnlyMount(path string) bool {
	for {
		var s unix.Statfs_t
		if err := unix.Statfs(path, &s); err == nil {
			return s.Flags&unix.ST_RDONLY != 0
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
