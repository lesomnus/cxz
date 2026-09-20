package core

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Closing the returned file releases the lock, including on process exit.
func Lock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errors.New("already running: " + path)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return f, nil
}
