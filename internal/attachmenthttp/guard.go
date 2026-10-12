package attachmenthttp

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"google.golang.org/grpc"
)

type UploaderFunc func(context.Context, assets.Upload, io.Reader) (string, error)

func (f UploaderFunc) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	return f(ctx, in, src)
}

// Reuse the mutation/version-switch guards while bytes remain a raw reader.
// No protobuf is marshaled and no gRPC call or chunk stream is opened here.
func Guard(uploader assets.Uploader, guards ...grpc.StreamServerInterceptor) assets.Uploader {
	return UploaderFunc(func(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
		var path string
		next := grpc.StreamHandler(func(_ any, _ grpc.ServerStream) error {
			var err error
			path, err = uploader.UploadAttachment(ctx, in, src)
			return err
		})
		info := &grpc.StreamServerInfo{FullMethod: "/cxz.SessionService/Upload", IsClientStream: true}
		for i := len(guards) - 1; i >= 0; i-- {
			guard, handler := guards[i], next
			next = func(srv any, stream grpc.ServerStream) error { return guard(srv, stream, info, handler) }
		}
		err := next(uploader, contextStream{ctx: ctx})
		return path, err
	})
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }
