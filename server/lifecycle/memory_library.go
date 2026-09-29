package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s SessionServer) Library(ctx context.Context, r *resource.SessionLibraryRequest) (*resource.SessionMemoryReply, error) {
	if e := s.effect(); e != nil {
		return nil, e
	}
	v, e := s.resolve(ctx, r.GetRef())
	if e != nil {
		return nil, e
	}
	client, ok := s.shared.runtime.(memorylib.Client)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "update manager for shared memory")
	}
	var q memorylib.Request
	if len(r.GetRequest()) > memorylib.MaxBundle+65536 {
		return nil, status.Error(codes.InvalidArgument, "request too large")
	}
	if e = json.Unmarshal(r.GetRequest(), &q); e != nil {
		return nil, e
	}
	out, e := client.Library(ctx, v.GetRuntimeId(), q)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(out)
	if e != nil {
		return nil, e
	}
	return resource.SessionMemoryReply_builder{Data: b}.Build(), nil
}
