package core

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// TreeBytes is what the files under a directory take up, as the filesystem
// reports it. A directory that is not there is zero rather than an error: it is
// how a session that has not written something yet reads. One that disappears
// while being walked is skipped for the same reason -- a live session writing
// is not a reason to refuse an answer about everything else.
func TreeBytes(ctx context.Context, dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}
