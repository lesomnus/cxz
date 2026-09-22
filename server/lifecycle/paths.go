package lifecycle

import (
	"context"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s ProjectServer) Paths(r *resource.ProjectPathsRequest, stream grpc.ServerStreamingServer[resource.ProjectPathsReply]) error {
	if err := s.effect(); err != nil {
		return err
	}
	path := r.GetPath()
	if len(path) > 4096 || strings.ContainsRune(path, 0) || !(strings.HasPrefix(path, "/") || path == "~" || strings.HasPrefix(path, "~/")) {
		return status.Error(codes.InvalidArgument, "absolute or home-relative directory required")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 4*time.Second)
	defer cancel()
	// Resolve the resource without an inventory refresh or trusting client-supplied
	// container/user metadata. The runtime checks live ownership and running state.
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if !p.GetListed() {
		return status.Error(codes.NotFound, "project deleted")
	}
	runtime, ok := s.shared.runtime.(containerterm.PathClient)
	if !ok {
		return status.Error(codes.Unimplemented, "container path browsing unavailable")
	}
	sent := 0
	var sendErr error
	emit := func(listing containerterm.PathListing) {
		if sendErr != nil {
			return
		}
		entries := make([]*resource.ProjectPathEntry, 0, len(listing.Entries)-sent)
		for _, e := range listing.Entries[sent:] {
			entries = append(entries, resource.ProjectPathEntry_builder{Name: &e.Name, Directory: &e.Directory, Executable: &e.Executable, Symlink: &e.Symlink, LinkTarget: &e.LinkTarget}.Build())
		}
		sendErr = stream.Send(resource.ProjectPathsReply_builder{Entries: entries, Truncated: &listing.Truncated}.Build())
		if sendErr != nil {
			cancel()
		}
		sent = len(listing.Entries)
	}
	out, err := runtime.Paths(ctx, p.GetRuntimeId(), path, emit)
	if sendErr != nil {
		return sendErr
	}
	if err != nil {
		return err
	}
	emit(out)
	return sendErr
}
