package resourceclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A file body is an HTTP request body, and that is the only thing it is.
//
// There is no probe here any more. Discovery existed so a client could stop
// before consuming the source and send it somewhere else instead; with nowhere
// else to send it, the probe was a round trip that bought a worse error. The
// upload carries `Expect: 100-continue`, so a Manager that does not serve the
// route answers before the body is read anyway.
func (c *Client) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.uploads == nil {
		// A connection with no way to reach the attachment endpoint: a unix
		// address that is not a Manager's daemon.sock, a relay whose credential
		// this client does not hold, or a scheme that has no data path. Saying
		// so is the whole answer -- there is no second path to try.
		return "", status.Error(codes.Unimplemented, "this connection cannot upload files; it has no attachment endpoint")
	}
	// The endpoint addresses a session by runtime id, so an alias is resolved
	// here rather than being sent as one.
	if sessionalias.Valid(in.SessionID) {
		v, err := c.sessions.Get(ctx, resource.SessionGetRequest_builder{Ref: sr(in.SessionID), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			return "", err
		}
		in.SessionID = v.GetRuntimeId()
	}
	return c.uploads.UploadAttachment(ctx, in, src)
}
