package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s ProjectServer) Download(r *resource.ProjectDownloadRequest, stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	if err := s.effect(); err != nil {
		return err
	}
	if !containerterm.ValidDownloadPath(r.GetPath()) {
		return status.Error(codes.InvalidArgument, "a file path is required")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 30*time.Minute)
	defer cancel()
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if !p.GetListed() {
		return status.Error(codes.NotFound, "project deleted")
	}
	runtime, ok := s.shared.runtime.(containerterm.DownloadClient)
	if !ok {
		return status.Error(codes.Unimplemented, "container downloads unavailable; update the manager")
	}
	return runtime.Download(ctx, p.GetRuntimeId(), r.GetPath(), downloadStreamWriter{stream})
}

type downloadStreamWriter struct {
	stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]
}

func (w downloadStreamWriter) Write(data []byte) (int, error) {
	total := 0
	for len(data) > 0 {
		n := min(len(data), 64*1024)
		if err := w.stream.Send(resource.ProjectDownloadReply_builder{Data: data[:n]}.Build()); err != nil {
			return total, err
		}
		total += n
		data = data[n:]
	}
	return total, nil
}
