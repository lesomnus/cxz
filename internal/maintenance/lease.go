package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

func VersionLock(root string) string { return filepath.Join(root, "version-maintenance.lock") }

// VersionLease serializes publication/launch with GC. A read-only project can
// lock the manager-created file. Missing locks on old read-only installations
// are tolerated: those managers do not run the new shared-tools collector.
func VersionLease(ctx context.Context, root string) (func(), error) {
	path := VersionLock(root)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = os.MkdirAll(root, 0755); err != nil {
			return func() {}, nil
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return func() {}, nil
		}
		f.Close()
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
