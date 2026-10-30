package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

func VersionLock(root string) string { return filepath.Join(root, "version-maintenance.lock") }

func lockHeld(path string) (bool, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() || !PlainPath(path) {
		return false, fmt.Errorf("unsafe process lease %s", path)
	}
	l := flock.New(path, flock.SetFlag(os.O_RDWR))
	defer l.Close()
	ok, err := l.TryLock()
	return !ok, err
}

// VersionLease serializes publication/launch with GC. A read-only project can
// lock the manager-created file. Missing locks on old read-only installations
// are tolerated: those managers do not run the new shared-tools collector.
func VersionLease(ctx context.Context, root string) (func(), error) {
	path := VersionLock(root)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = os.MkdirAll(root, 0755); err != nil {
			if readOnlyMount(root) {
				return func() {}, nil
			}
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			if readOnlyMount(root) {
				return func() {}, nil
			}
			return nil, err
		}
		f.Close()
	}
	if st, err := os.Lstat(path); err != nil {
		return nil, err
	} else if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid executable publication lease %s", path)
	}
	l := flock.New(path, flock.SetFlag(os.O_RDONLY))
	ok, err := l.TryRLockContext(ctx, 25*time.Millisecond)
	if err != nil || !ok {
		l.Close()
		if err == nil {
			err = ctx.Err()
		}
		return nil, err
	}
	return func() { l.Close() }, nil
}
