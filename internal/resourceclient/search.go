package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

// Search is the one call on this client that is about no session in particular,
// so there is no ref to build and nothing to decorate on the way out: the
// manager has already said which project and which session each result came
// from, because only it knows.
func (c *Client) Search(ctx context.Context, r *api.SearchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.SearchReply], error) {
	s, err := c.sessions.Search(ctx, resource.SessionSearchRequest_builder{
		Query: &r.Query, Match: &r.Match, IgnoreCase: &r.IgnoreCase,
		View: &r.View, IncludeTools: &r.IncludeTools,
		SinceMs: &r.SinceMs, UntilMs: &r.UntilMs,
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
			ActivityMs: v.GetActivityMs(), CreatedMs: v.GetCreatedMs(), Truncated: v.GetTruncated(),
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
		out.Summary = &api.SearchSummary{Projects: v.GetProjects(), Unavailable: v.GetUnavailable(), Sessions: v.GetSessions(), Hits: v.GetHits(), Truncated: v.GetTruncated(), NextCursor: v.GetNextCursor(), HasMore: v.GetHasMore(), SinceMs: v.GetSinceMs(), UntilMs: v.GetUntilMs()}
	}
	return out, nil
}
