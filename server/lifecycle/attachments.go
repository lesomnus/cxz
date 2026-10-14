package lifecycle

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s SessionServer) Upload(stream grpc.ClientStreamingServer[resource.SessionUploadRequest, resource.SessionAttachment]) error {
	header, err := stream.Recv()
	if err != nil {
		return err
	}
	if header.GetRef() == nil || header.GetName() == "" || len(header.GetContent()) != 0 {
		return status.Error(codes.InvalidArgument, "file upload must start with a header")
	}
	ctx := stream.Context()
	path, err := s.upload(ctx, header.GetRef(), assets.Upload{
		RunID: header.GetRunId(), Name: header.GetName(), Size: header.GetSize(),
	}, &attachmentReader{stream: stream})
	if err != nil {
		return err
	}
	return stream.SendAndClose(resource.SessionAttachment_builder{Path: &path}.Build())
}

// Both wire paths retain the same lifecycle lock, resolved session and runtime
// run/mount checks. Only their io.Reader adapters differ.
func (s Layer) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	return (SessionServer{Layer: s, SessionServiceServer: s.Next().Session()}).upload(ctx, sessionRef(in.SessionID), in, src)
}
func (s SessionServer) upload(ctx context.Context, ref *resource.SessionRef, in assets.Upload, src io.Reader) (string, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return "", err
	}
	v, err := s.resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	in.SessionID = v.GetRuntimeId()
	return s.shared.runtime.UploadAttachment(ctx, in, src)
}

// Adapt the live gRPC stream to the reader consumed by flob.Add. No upload
// session, offsets, locks or recovery files exist outside this call.
type attachmentReader struct {
	stream  grpc.ClientStreamingServer[resource.SessionUploadRequest, resource.SessionAttachment]
	pending []byte
}

func (r *attachmentReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 {
		message, err := r.stream.Recv()
		if err != nil {
			return 0, err
		}
		if message.HasRef() || message.HasRunId() || message.HasName() || message.HasSize() {
			return 0, status.Error(codes.InvalidArgument, "upload header must only appear once")
		}
		r.pending = message.GetContent()
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

var _ io.Reader = (*attachmentReader)(nil)
