package resourceclient

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Diagnostics reads what the manager has been doing. There is no runtime-API
// counterpart to translate to: the manager's RPC log is the resource layer's
// own account of itself. See cxz#211.
func (c *Client) Diagnostics(ctx context.Context, method string, since time.Time, limit int32, statsOnly bool, opts ...grpc.CallOption) (*resource.DiagnosticsReply, error) {
	r := resource.DiagnosticsRequest_builder{Method: &method, Limit: &limit, StatsOnly: &statsOnly}.Build()
	if !since.IsZero() {
		r.SetSince(timestamppb.New(since))
	}
	return c.projects.Diagnostics(ctx, r, opts...)
}
func (c *Client) CaptureDiagnostics(ctx context.Context, d time.Duration, opts ...grpc.CallOption) (*resource.DiagnosticsReply, error) {
	seconds := int32(d / time.Second)
	return c.projects.CaptureDiagnostics(ctx, resource.CaptureDiagnosticsRequest_builder{Seconds: &seconds}.Build(), opts...)
}
