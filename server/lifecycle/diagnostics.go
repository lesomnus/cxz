package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/observe"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The manager's own RPC log, which used to be reachable only as container
// stdout on the installation host. Metadata only: this answer crosses the
// plaintext surface, so no request or reply is in it. See cxz#211.
func (s ProjectServer) Diagnostics(_ context.Context, r *resource.DiagnosticsRequest) (*resource.DiagnosticsReply, error) {
	o := s.shared.observe
	if o == nil {
		return nil, status.Error(codes.Unavailable, "diagnostics are not collected by this server")
	}
	out := diagnostics(o)
	if r.GetStatsOnly() {
		return out, nil
	}
	var since time.Time
	if r.HasSince() {
		since = r.GetSince().AsTime()
	}
	for _, v := range o.Records(since, r.GetMethod(), int(r.GetLimit())) {
		out.SetRecords(append(out.GetRecords(), record(v)))
	}
	return out, nil
}

// A capture window expires on its own, so an installation cannot be left
// writing every call forever.
func (s ProjectServer) CaptureDiagnostics(_ context.Context, r *resource.CaptureDiagnosticsRequest) (*resource.DiagnosticsReply, error) {
	o := s.shared.observe
	if o == nil {
		return nil, status.Error(codes.Unavailable, "diagnostics are not collected by this server")
	}
	if r.GetSeconds() < 0 {
		return nil, status.Error(codes.InvalidArgument, "a capture window cannot be negative")
	}
	o.Capture(time.Duration(r.GetSeconds()) * time.Second)
	return diagnostics(o), nil
}

func diagnostics(o *observe.Log) *resource.DiagnosticsReply {
	out := resource.DiagnosticsReply_builder{Path: ptr(o.File())}.Build()
	for _, v := range o.Stats() {
		out.SetStats(append(out.GetStats(), resource.RpcStat_builder{
			Method: &v.Method, Calls: &v.Calls, Failed: &v.Failed, InFlight: &v.InFlight,
			P50Ms: &v.P50Ms, P99Ms: &v.P99Ms, MaxMs: &v.MaxMs,
		}.Build()))
	}
	if until := o.CapturingUntil(); !until.IsZero() {
		out.SetCapturingUntil(timestamppb.New(until))
	}
	return out
}
func record(v observe.Record) *resource.RpcRecord {
	return resource.RpcRecord_builder{
		At: timestamppb.New(v.At), Method: &v.Method, Code: &v.Code,
		ElapsedMs: &v.ElapsedMs, WaitedMs: &v.WaitedMs, WaitedOn: &v.WaitedOn, Stream: &v.Stream,
	}.Build()
}
