package lifecycle

import (
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
	session, err := s.resolve(ctx, header.GetRef())
	if err != nil {
		return err
	}
	path, err := s.shared.runtime.UploadAttachment(ctx, assets.Upload{
		SessionID: session.GetRuntimeId(), RunID: header.GetRunId(), Name: header.GetName(), Size: header.GetSize(),
	}, &attachmentReader{stream: stream})
	if err != nil {
		return err
	}
	return stream.SendAndClose(resource.SessionAttachment_builder{Path: &path}.Build())
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
