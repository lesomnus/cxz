package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Attach(ctx context.Context, r *api.AttachmentInput) (*api.Attachment, error) {
	if len(r.Content) == 0 || len(r.Content) > 1024*1024 {
		return nil, status.Error(codes.InvalidArgument, "attachment must contain 1 byte to 1 MiB")
	}
	if s.manager != nil {
		conn, client, err := s.manager.ClientFor(ctx, r.SessionId)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return client.Attach(ctx, r)
	}
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	v, err := s.snapshot(ctx, m)
	if err != nil {
		return nil, err
	}
	if r.RunId == "" || r.RunId != v.RunId {
		return nil, status.Error(codes.FailedPrecondition, "session run changed; retry from the current session")
	}
	path, err := storeAttachment(core.Dir(s.root, m.ID), r.Content)
	if err != nil {
		return nil, status.Error(codes.Internal, "cannot persist session attachment")
	}
	return &api.Attachment{Path: path}, nil
}

// Content-addressed, immutable, private files in the session's durable volume.
// Root confines symlink traversal; callers cannot choose filesystem paths.
func storeAttachment(dir string, content []byte) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err = root.Mkdir("attachments", 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	name := fmt.Sprintf("attachments/paste-%x.txt", sha256.Sum256(content))
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		old, e := root.Open(name)
		if e != nil {
			return "", e
		}
		defer old.Close()
		b, e := io.ReadAll(io.LimitReader(old, 1024*1024+1))
		if e != nil || !bytes.Equal(b, content) {
			return "", fmt.Errorf("attachment collision or corruption")
		}
		return filepath.Join(dir, name), nil
	}
	if err != nil {
		return "", err
	}
	_, err = f.Write(content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(name) // Only the new file created by this attempt.
		return "", err
	}
	return filepath.Join(dir, name), nil
}
