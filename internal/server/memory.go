package server

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memoryview"
)

func (s *Server) Memory(ctx context.Context, r *api.MemoryRequest) (*api.MemoryReply, error) {
	if s.manager != nil {
		return s.manager.Memory(ctx, r)
	}
	selected, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	page, err := memoryview.Read(ctx, s.root, memoryview.Query{Session: selected, Path: r.Path})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(page)
	return &api.MemoryReply{Data: data}, err
}

func (s *Server) CopyMemory(ctx context.Context, r *api.CopyMemoryRequest) (*api.Receipt, error) {
	if s.manager != nil {
		return s.manager.CopyMemory(ctx, r)
	}
	source, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	target, err := s.manifest(ctx, r.TargetId)
	if err != nil {
		return nil, err
	}
	message, err := memoryview.Copy(ctx, s.root, s.root, memoryview.CopyQuery{Source: source, Target: target, Path: r.Path, TargetPath: r.TargetPath})
	return &api.Receipt{Status: message}, err
}
