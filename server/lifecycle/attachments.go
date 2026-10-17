package lifecycle

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/resource"
)

// One wire path now, so there is one adapter and no choice to make: the raw
// HTTP handler hands this an io.Reader, and what happens below is the lifecycle
// lock, the resolved session and the runtime's run and mount checks.
func (s Layer) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	return (SessionServer{Layer: s, SessionServiceServer: s.Next().Session()}).upload(ctx, sessionRef(in.SessionID), in, src)
}
func (s SessionServer) upload(ctx context.Context, ref *resource.SessionRef, in assets.Upload, src io.Reader) (string, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return "", err
	}
	v, err := s.resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	in.SessionID = v.GetRuntimeId()
	return s.shared.runtime.UploadAttachment(ctx, in, src)
}
