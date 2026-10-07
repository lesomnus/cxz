package resourceclient

import (
	"context"

	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Search is the one call on this client that is about no session in particular,
// so there is no ref to build and nothing to decorate on the way out: the
// manager has already said which project and which session each result came
// from, because only it knows.
func (c *Client) Search(ctx context.Context, r *api.SearchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.SearchReply], error) {
	s, err := c.sessions.Search(ctx, resource.SessionSearchRequest_builder{
		Query: &r.Query, Match: &r.Match, IgnoreCase: &r.IgnoreCase,
		View: &r.View, IncludeTools: &r.IncludeTools,
		Since: bound(r.SinceMs), Until: bound(r.UntilMs),
		Projects: r.Projects, Exclude: r.Exclude, Sessions: r.Sessions,
		Limit: &r.Limit, Snippet: &r.Snippet, Cursor: &r.Cursor, ClientId: &r.ClientId,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return searchStream{s}, nil
}

type searchStream struct {
	grpc.ServerStreamingClient[resource.SessionSearchReply]
}

// bound leaves an absent window absent rather than sending the epoch: the wire
// says "no lower bound" by having nothing there.
func bound(ms int64) *timestamppb.Timestamp {
	if ms == 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(ms).UTC())
}

func boundMS(t *timestamppb.Timestamp) int64 {
	if t == nil {
		return 0
	}
	return t.AsTime().UnixMilli()
}

func (s searchStream) Recv() (*api.SearchReply, error) {
	in, err := s.ServerStreamingClient.Recv()
	if err != nil {
		return nil, err
	}
	out := &api.SearchReply{}
	if v := in.GetVisit(); v != nil {
		visit := &api.SearchVisit{
			ProjectId: v.GetProjectId(), ProjectName: v.GetProjectName(),
			SessionId: v.GetSessionId(), Alias: v.GetAlias(), Title: v.GetTitle(),
			Agent: v.GetAgent(), State: v.GetState(),
			ActivityMs: v.GetActivityMs(), CreatedMs: v.GetCreatedMs(), Truncated: v.GetTruncated(), Approximate: v.GetApproximate(),
		}
		for _, h := range v.GetHits() {
			visit.Hits = append(visit.Hits, &api.SearchHit{Seq: h.GetSeq(), TimeMs: h.GetTimeMs(), Kind: h.GetKind(), Bytes: h.GetBytes(), Score: h.GetScore(), Snippet: h.GetSnippet()})
		}
		out.Visit = visit
	}
	if p := in.GetProgress(); p != nil {
		out.Progress = &api.SearchProgress{ProjectId: p.GetProjectId(), ProjectName: p.GetProjectName(), State: p.GetState(), Message: p.GetMessage(), Opened: p.GetOpened(), Total: p.GetTotal()}
	}
	if v := in.GetSummary(); v != nil {
		out.Summary = &api.SearchSummary{Projects: v.GetProjects(), Unavailable: v.GetUnavailable(), Sessions: v.GetSessions(), Hits: v.GetHits(), Truncated: v.GetTruncated(), NextCursor: v.GetNextCursor(), HasMore: v.GetHasMore(), SinceMs: boundMS(v.GetSince()), UntilMs: boundMS(v.GetUntil())}
	}
	return out, nil
}
