package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

func (s SessionServer) Storage(ctx context.Context, r *resource.SessionRef) (*resource.SessionStorageReply, error) {
	v, err := s.resolve(ctx, r)
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.SessionStorage(ctx, &api.SessionRef{Id: v.GetRuntimeId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionStorageReply_builder{
		JournalBytes:  &out.JournalBytes,
		ProviderBytes: &out.ProviderBytes,
		ProfileBytes:  &out.ProfileBytes,
		TotalBytes:    &out.TotalBytes,
	}.Build(), nil
}
