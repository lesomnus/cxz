package server

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func (s *Server) Open(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
	if s.manager == nil {
		return nil, status.Error(codes.PermissionDenied, "project lifecycle is only available through the installed manager")
	}
	return s.manager.Open(ctx, r)
}
func (s *Server) Projects(ctx context.Context, _ *api.Empty) (*api.ProjectList, error) {
	if s.manager == nil {
		return nil, status.Error(codes.PermissionDenied, "project lifecycle is not available in this runtime")
	}
	return s.manager.Projects(ctx)
}
func (s *Server) Down(ctx context.Context, r *api.ProjectRequest) (*api.Receipt, error) {
	if s.manager == nil {
		return nil, status.Error(codes.PermissionDenied, "project lifecycle is not available in this runtime")
	}
	if e := s.manager.Down(ctx, r); e != nil {
		return nil, e
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "stopped; workspace and named volumes retained"}, nil
}
func (s *Server) watchRemote(r *api.WatchRequest, stream grpc.ServerStreamingServer[api.Event]) error {
	if _, e := s.manager.Get(stream.Context(), r.SessionId); e != nil {
		return e
	}
	cursor := r.AfterSeq
	clientID := r.ClientId
	if clientID == "" {
		clientID = "legacy-" + core.ID()
	}
	var lastActivity time.Time
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if lastActivity.IsZero() || (r.ClientId == "" && time.Since(lastActivity) >= 10*time.Second) {
			if session, err := s.manager.Get(stream.Context(), r.SessionId); err == nil {
				ctx, cancel := context.WithTimeout(stream.Context(), 3*time.Second)
				_, _ = s.Activity(ctx, &api.ActivityInput{SessionId: r.SessionId, RunId: session.RunId, ClientId: clientID, Busy: true})
				cancel()
			}
			lastActivity = time.Now()
		}
		batch, e := s.manager.History(stream.Context(), &api.WatchRequest{SessionId: r.SessionId, AfterSeq: cursor})
		if e != nil {
			return e
		}
		for _, v := range batch.Events {
			if e = stream.Send(v); e != nil {
				return e
			}
			cursor = v.Seq
		}
		if len(batch.Events) == 128 {
			continue
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-ticker.C:
		}
	}
}
