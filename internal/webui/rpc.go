// Package webui serves a browser transport for the existing Manager.
package webui

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Keep the initial browser surface explicit. Unimplemented methods fail closed;
// no second database or lifecycle implementation is opened by this gateway.
type projects struct {
	resource.UnimplementedProjectServiceServer
	client  resource.ProjectServiceClient
	editors *editorProxy
}

func (s *projects) List(ctx context.Context, r *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	return s.client.List(ctx, r)
}
func (s *projects) Get(ctx context.Context, r *resource.ProjectGetRequest) (*resource.Project, error) {
	return s.client.Get(ctx, r)
}

func (s *projects) workspacePath(ctx context.Context, ref *resource.ProjectRef, requested string) error {
	p, err := s.client.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: boolPointer(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	root := p.GetStatus().GetRemoteWorkspace()
	if root == "" || !strings.HasPrefix(root, "/") {
		return status.Error(codes.FailedPrecondition, "project workspace unavailable")
	}
	root = path.Clean(root)
	requested = path.Clean(requested)
	if requested != root && !strings.HasPrefix(requested, strings.TrimSuffix(root, "/")+"/") {
		return status.Error(codes.PermissionDenied, "path outside project workspace")
	}
	return nil
}
func boolPointer(v bool) *bool { return &v }

func (s *projects) Paths(r *resource.ProjectPathsRequest, stream grpc.ServerStreamingServer[resource.ProjectPathsReply]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	if err := s.workspacePath(ctx, r.GetRef(), r.GetPath()); err != nil {
		return err
	}
	upstream, err := s.client.Paths(ctx, r)
	if err != nil {
		return err
	}
	for {
		v, err := upstream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(v); err != nil {
			return err
		}
	}
}
func (s *projects) Download(r *resource.ProjectDownloadRequest, stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	if err := s.workspacePath(ctx, r.GetRef(), r.GetPath()); err != nil {
		return err
	}
	upstream, err := s.client.Download(ctx, r)
	if err != nil {
		return err
	}
	total := 0
	for {
		v, err := upstream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		total += len(v.GetData())
		if total > 1<<20 || v.GetTotalSize() > 1<<20 {
			return status.Error(codes.ResourceExhausted, "file preview is limited to 1 MiB")
		}
		if err = stream.Send(v); err != nil {
			return err
		}
	}
}
func (s *projects) Editor(ctx context.Context, r *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	return s.editors.connect(ctx, r)
}

type sessions struct {
	resource.UnimplementedSessionServiceServer
	client resource.SessionServiceClient
}

func (s *sessions) List(ctx context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	return s.client.List(ctx, r)
}
func (s *sessions) Get(ctx context.Context, r *resource.SessionGetRequest) (*resource.Session, error) {
	return s.client.Get(ctx, r)
}
func (s *sessions) History(ctx context.Context, r *resource.SessionEventsRequest) (*resource.SessionEventBatch, error) {
	return s.client.History(ctx, r)
}
func (s *sessions) Send(ctx context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	return s.client.Send(ctx, r)
}
func (s *sessions) Reply(ctx context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	return s.client.Reply(ctx, r)
}
func (s *sessions) Interrupt(ctx context.Context, r *resource.SessionControl) (*resource.SessionReceipt, error) {
	return s.client.Interrupt(ctx, r)
}
func (s *sessions) Stop(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	return s.client.Stop(ctx, r)
}
func (s *sessions) Resume(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	return s.client.Resume(ctx, r)
}
func (s *sessions) Permission(ctx context.Context, r *resource.SessionPermissionRequest) (*resource.SessionReceipt, error) {
	return s.client.Permission(ctx, r)
}
func (s *sessions) Events(r *resource.SessionEventsRequest, stream grpc.ServerStreamingServer[resource.SessionEvent]) error {
	upstream, err := s.client.Events(stream.Context(), r)
	if err != nil {
		return err
	}
	for {
		e, err := upstream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(e); err != nil {
			return err
		}
	}
}

func (s *projects) Watch(r *resource.ProjectWatchRequest, stream grpc.ServerStreamingServer[resource.ProjectWatchResponse]) error {
	upstream, err := s.client.Watch(stream.Context(), r)
	if err != nil {
		return err
	}
	for {
		e, err := upstream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(e); err != nil {
			return err
		}
	}
}
func (s *sessions) Watch(r *resource.SessionWatchRequest, stream grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	upstream, err := s.client.Watch(stream.Context(), r)
	if err != nil {
		return err
	}
	for {
		e, err := upstream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(e); err != nil {
			return err
		}
	}
}

func (s *sessions) Transcript(ctx context.Context, r *resource.SessionTranscriptRequest) (*resource.SessionTranscriptReply, error) {
	return s.client.Transcript(ctx, r)
}
func (s *sessions) EventDetails(ctx context.Context, r *resource.SessionEventDetailsRequest) (*resource.SessionEventBatch, error) {
	return s.client.EventDetails(ctx, r)
}

func (s *sessions) Models(ctx context.Context, r *resource.SessionModelsRequest) (*resource.SessionModelsReply, error) {
	return s.client.Models(ctx, r)
}
