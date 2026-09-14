//go:build linux

package wisp

import (
	"encoding/hex"
	"golang.org/x/sys/unix"
	"os"
	"time"
)

func CheckSecretRoot(root string) error {
	fd, err := checkedSecretRoot(root)
	if err == nil {
		unix.Close(fd)
	}
	return err
}

// Sweep only cxz's short random directories; never recurse or follow symlinks.
// Reading directory metadata does not refresh the secret's access timestamp.
func SweepSecrets(root string, now time.Time) error {
	fd, err := checkedSecretRoot(root)
	if err != nil {
		return err
	}
	return sweepSecretFD(fd, now)
}
func sweepSecretFD(fd int, now time.Time) error {
	directory := os.NewFile(uintptr(fd), "secret-root")
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) != 8 || !entry.IsDir() {
			continue
		}
		if _, err := hex.DecodeString(name); err != nil {
			continue
		}
		dir, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		var st unix.Stat_t
		statErr := unix.Fstatat(dir, "value", &st, unix.AT_SYMLINK_NOFOLLOW)
		if statErr == nil && st.Mode&unix.S_IFMT == unix.S_IFREG {
			last := time.Unix(st.Atim.Sec, st.Atim.Nsec)
			modified := time.Unix(st.Mtim.Sec, st.Mtim.Nsec)
			if modified.After(last) {
				last = modified
			}
			if !last.After(now.Add(-DefaultSecretMaxIdle)) {
				_ = unix.Unlinkat(dir, "value", 0)
				_ = unix.Unlinkat(fd, name, unix.AT_REMOVEDIR)
			}
		} else if statErr == unix.ENOENT && unix.Fstat(dir, &st) == nil && !time.Unix(st.Mtim.Sec, st.Mtim.Nsec).After(now.Add(-DefaultSecretMaxIdle)) {
			// An interrupted allocation may leave an empty directory. rmdir
			// fails harmlessly if it contains anything unrelated.
			_ = unix.Unlinkat(fd, name, unix.AT_REMOVEDIR)
		}
		unix.Close(dir)
	}
	return nil
}
