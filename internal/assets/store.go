//go:build !js

package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/flob"
)

func ExportRoot(root, project string) string { return filepath.Join(root, "exports", project) }
func NewStores(root string) flob.OsStores    { return flob.NewOsStores(filepath.Join(root, "cas")) }

// Add streams directly into flob, then publishes the original filename as a
// hard link. Each attachment has its own directory, so names never collide.
// Neither filename labels nor attachment sidecars are needed.
func Add(ctx context.Context, root, project, session, name string, size int64, src io.Reader) (string, error) {
	if !ValidID(project) || !ValidID(session) {
		return "", fmt.Errorf("invalid attachment scope")
	}
	if !ValidName(name) {
		return "", fmt.Errorf("invalid attachment filename")
	}
	if size < 0 || size > MaxSize {
		return "", fmt.Errorf("invalid attachment size")
	}
	store := NewStores(root).Use(session)
	meta, err := store.Add(ctx, flob.Meta{}, &sizedReader{ctx: ctx, src: src, remaining: size})
	if err != nil && !errors.Is(err, flob.ErrAlreadyExists) {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	reader, _, err := store.Open(ctx, meta.Digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	// flob's pinned OS backend returns *os.File. Obtain the path here without
	// reconstructing its private digest/sharding layout.
	file, ok := reader.(*os.File)
	if !ok {
		return "", fmt.Errorf("asset storage does not expose local files")
	}
	if err := file.Chmod(0444); err != nil {
		return "", err
	}
	parent := filepath.Join(ExportRoot(root, project), session)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	dir := filepath.Join(parent, core.ID())
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	if err := os.Link(file.Name(), dest); err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	return dest, nil
}

// Forget removes one session's uploads: the published links first, then every
// blob the session's flob namespace claims. Order matters because an export is
// a hard link to the blob, so the blob cannot be the last reference until the
// link is gone. flob reclaims a shared blob only when the namespace letting go
// held its final reference, so a file two sessions uploaded survives the first
// purge and disappears with the second.
func Forget(ctx context.Context, root, project, session string) error {
	if !ValidID(project) || !ValidID(session) {
		return fmt.Errorf("invalid attachment scope")
	}
	if err := os.RemoveAll(filepath.Join(ExportRoot(root, project), session)); err != nil {
		return err
	}
	store := NewStores(root).Use(session)
	walker, ok := flob.AsWalker(store)
	if !ok {
		return fmt.Errorf("asset storage cannot inventory a session's blobs")
	}
	var digests []flob.Digest
	for info, err := range walker.Walk(ctx) {
		if err != nil {
			// A namespace that never received an upload has nothing to walk.
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		digests = append(digests, info.Digest())
	}
	for _, d := range digests {
		if err := store.Erase(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// Reject short, oversized or cancelled streams before flob commits any blob.
type sizedReader struct {
	ctx       context.Context
	src       io.Reader
	remaining int64
}

func (r *sizedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	p = p[:min(int64(len(p)), r.remaining+1)]
	n, err := r.src.Read(p)
	r.remaining -= int64(n)
	if r.remaining < 0 {
		return n, fmt.Errorf("%w: exceeds declared size", ErrSizeMismatch)
	}
	if err == io.EOF && r.remaining != 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// RemoveSession drops this session's attachment references. Other sessions' links
// keep shared blobs alive; flob reclaims blobs with no remaining references.
func RemoveSession(ctx context.Context, root, project, session string) error {
	if !ValidID(project) || !ValidID(session) {
		return fmt.Errorf("invalid attachment scope")
	}
	r, err := os.OpenRoot(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.RemoveAll(filepath.Join("exports", project, session)); err != nil {
		return err
	}
	store := NewStores(root).Use(session).(flob.OsStore)
	for info, err := range store.Walk(ctx) {
		if err != nil {
			return err
		}
		if err := store.Erase(ctx, info.Digest()); err != nil {
			return err
		}
	}
	return nil
}
