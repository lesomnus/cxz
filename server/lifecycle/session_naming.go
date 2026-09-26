package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/protobuf-orm/ent/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Payday's default alias grammar is for DNS labels. Sessions use their own
// grammar; keep the same bare store, scope, audit recorder and watch recorder.
type sessionNamingSink struct{ pd.Sink }

func (s sessionNamingSink) Session() resource.SessionServiceServer {
	return sessionNamingStore{s.Sink.Session(), s.Sink.Server.Session()}
}

func (s sessionNamingSink) WithDriver(d dialect.Driver) (resource.Server, error) {
	next, err := s.Sink.WithDriver(d)
	if err != nil {
		return nil, err
	}
	return sessionNamingSink{next.(pd.Sink)}, nil
}

type sessionNamingStore struct {
	resource.SessionServiceServer
	storage resource.SessionServiceServer
}

func (s sessionNamingStore) Patch(ctx context.Context, r *resource.SessionPatchRequest) (*resource.Session, error) {
	if r.HasAlias() && !sessionalias.Valid(r.GetAlias()) {
		return nil, status.Error(codes.InvalidArgument, sessionalias.Rule)
	}
	return s.storage.Patch(ctx, r)
}
