package memorylib

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// SweepTrash never reads or removes live documents. An installation upgrading
// from trash without deletion timestamps receives a full grace period first.
func SweepTrash(ctx context.Context, root string, now time.Time, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	projects, err := os.ReadDir(filepath.Join(root, "memories"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if !project.IsDir() {
			continue
		}
		library := filepath.Join(root, "memories", project.Name())
		n, err := sweepLibrary(ctx, library, now, time.Duration(days)*24*time.Hour, 128-removed)
		removed += n
		if err != nil {
			return removed, err
		}
		if removed >= 128 {
			break
		}
	}
	return removed, nil
}

func sweepLibrary(ctx context.Context, path string, now time.Time, age time.Duration, limit int) (int, error) {
	l := flock.New(filepath.Join(path, ".lock"))
	defer l.Close()
	ok, err := l.TryLock()
	if err != nil || !ok {
		return 0, err
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	barrier, err := r.Stat(".retention-start")
	if os.IsNotExist(err) {
		f, e := r.OpenFile(".retention-start", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		if e = f.Close(); e != nil {
			return 0, e
		}
		barrier, err = r.Stat(".retention-start")
	}
	if err != nil {
		return 0, err
	}
	f, err := r.Open(".")
	if err != nil {
		return 0, err
	}
	bundles, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return 0, err
	}
	removed := 0
	old := func(st os.FileInfo) bool {
		since := st.ModTime()
		if barrier.ModTime().After(since) {
			since = barrier.ModTime()
		}
		return now.Sub(since) >= age
	}
	for _, bundle := range bundles {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if removed >= limit {
			break
		}
		if !bundle.IsDir() {
			continue
		}
		if bundle.Name() == "trash" {
			f, err := r.Open("trash")
			if err != nil {
				return removed, err
			}
			es, err := f.ReadDir(-1)
			f.Close()
			if err != nil {
				return removed, err
			}
			for _, e := range es {
				if err := ctx.Err(); err != nil {
					return removed, err
				}
				if removed >= limit {
					break
				}
				st, err := e.Info()
				if err != nil || !e.IsDir() || !validID.MatchString(e.Name()) || !old(st) {
					continue
				}
				if err = r.RemoveAll("trash/" + e.Name()); err != nil {
					return removed, err
				}
				removed++
			}
			continue
		}
		if !validID.MatchString(bundle.Name()) {
			continue
		}
		f, err := r.Open(bundle.Name())
		if err != nil {
			return removed, err
		}
		docs, err := f.ReadDir(-1)
		f.Close()
		if err != nil {
			return removed, err
		}
		for _, doc := range docs {
			if err := ctx.Err(); err != nil {
				return removed, err
			}
			if removed >= limit {
				break
			}
			st, err := doc.Info()
			if err != nil || !st.Mode().IsRegular() || !deletedDoc(doc.Name()) || !old(st) {
				continue
			}
			if err = r.Remove(bundle.Name() + "/" + doc.Name()); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}
