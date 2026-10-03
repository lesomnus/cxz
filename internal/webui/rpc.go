// Package webui serves a browser transport for the existing Manager.
package webui

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

// Keep the initial browser surface explicit. Unimplemented methods fail closed;
// no second database or lifecycle implementation is opened by this gateway.
type projects struct {
	resource.UnimplementedProjectServiceServer
	client resource.ProjectServiceClient
}

func (s *projects) List(ctx context.Context, r *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	return s.client.List(ctx, r)
}
func (s *projects) Get(ctx context.Context, r *resource.ProjectGetRequest) (*resource.Project, error) {
	return s.client.Get(ctx, r)
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
