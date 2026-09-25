// Package assets publishes named hard links to flob content-addressed files.
package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/flob"
)

const MaxSize int64 = 1024 * 1024 * 1024
const MountPath = "/cxz/assets"

// Upload describes a single live request. It is never persisted as metadata.
type Upload struct {
	SessionID, RunID, Name string
	Size                   int64
}

type Uploader interface {
	UploadAttachment(context.Context, Upload, io.Reader) (string, error)
}

func ValidID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func ExportRoot(root, project string) string { return filepath.Join(root, "exports", project) }
func NewStores(root string) flob.OsStores    { return flob.NewOsStores(filepath.Join(root, "cas")) }

// Add streams directly into flob, then publishes the original filename as a
// hard link. Each attachment has its own directory, so names never collide.
// Neither filename labels nor attachment sidecars are needed.
func Add(ctx context.Context, root, project, session, name string, size int64, src io.Reader) (string, error) {
	if !ValidID(project) || !ValidID(session) {
		return "", fmt.Errorf("invalid attachment scope")
	}
	if name == "" || len(name) > 255 || !utf8.ValidString(name) || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) {
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
		return n, fmt.Errorf("attachment exceeds declared size")
	}
	if err == io.EOF && r.remaining != 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}
