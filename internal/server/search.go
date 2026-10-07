package server

import (
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/convindex"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Search answers for every conversation the installation holds.
//
// There is one answer for both kinds of installation now, which is what keeping
// the conversation as an index buys: a manager's projects and a host-local state
// directory differ in where their journals are, and not at all in where their
// conversation is. Nothing is fanned out, no project is opened to be read, and
// the reply is one query's result rather than a merge of several readers'.
func (s *Server) Search(r *api.SearchRequest, stream grpc.ServerStreamingServer[api.SearchReply]) error {
	ctx := stream.Context()
	if err := s.requireManagerAuthority(ctx, "search conversations from the host client, not from inside a project"); err != nil {
		return err
	}
	if s.conversations == nil {
		return status.Error(codes.Unavailable, "this installation keeps no conversation index")
	}
	q := convindex.Query{
		Query:        r.Query,
		Match:        r.Match,
		IgnoreCase:   r.IgnoreCase,
		IncludeTools: r.IncludeTools,
		Since:        atMS(r.SinceMs),
		Until:        atMS(r.UntilMs),
		Projects:     r.Projects,
		Exclude:      r.Exclude,
		Sessions:     r.Sessions,
		Snippet:      int(r.Snippet),
		Limit:        int(r.Limit),
	}
	if r.Cursor != "" {
		since, until, resume, err := convindex.DecodeCursor(r.Cursor)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		// The window is fixed while it is being paged through: events arriving
		// now are above it, and must not shift what has already been read.
		q.Since, q.Until, q.Resume = since, until, resume
	}
	// Ingestion catches up before the question is answered, so that a search
	// sees a conversation that is still happening. It is bounded: what it
	// cannot reach in time is reported rather than waited for.
	caught := s.catchUp(ctx)
	pending := 0
	for _, p := range caught {
		// Having been caught up is not being behind. Only what the index could
		// not reach makes an answer incomplete.
		if p.State != "indexed" {
			pending++
		}
	}
	for _, p := range caught {
		if err := stream.Send(&api.SearchReply{Progress: &api.SearchProgress{
			ProjectId: p.Project, ProjectName: p.Name, State: p.State, Message: p.Message,
			Done: int32(p.Done), Total: int32(p.Total),
		}}); err != nil {
			return err
		}
	}
	visits, out, err := s.conversations.Search(ctx, q, time.Now().UTC())
	if err != nil {
		return searchError(err)
	}
	projects := map[string]bool{}
	for _, v := range visits {
		projects[v.Project] = true
		reply := &api.SearchVisit{
			// The project's name and the session's alias belong to the layer
			// that owns them, and it adds them on the way out: the runtime
			// knows a session by the identity its journal is written under.
			ProjectId: v.Project, SessionId: v.Session, Title: v.Title, Agent: v.Agent,
			ActivityMs: msAt(v.Activity), CreatedMs: msAt(v.CreatedAt), Truncated: v.Trimmed,
		}
		for _, h := range v.Hits {
			reply.Hits = append(reply.Hits, &api.SearchHit{
				Seq: h.Seq, TimeMs: msAt(h.Time), Kind: h.Kind,
				Bytes: int32(h.Bytes), Score: int32(h.Score), Snippet: h.Snippet,
			})
		}
		if err = stream.Send(&api.SearchReply{Visit: reply}); err != nil {
			return err
		}
	}
	summary := &api.SearchSummary{
		Projects: int32(len(projects)), Sessions: int32(out.Sessions), Hits: int32(out.Hits),
		Truncated: int32(out.Trimmed), Examined: int32(out.Examined), Pending: int32(pending),
		SinceMs: msAt(out.Since), UntilMs: msAt(out.Until), HasMore: out.HasMore,
	}
	if out.HasMore && out.Next != nil {
		summary.NextCursor = convindex.EncodeCursor(out.Since, out.Until, *out.Next)
	}
	return stream.Send(&api.SearchReply{Summary: summary})
}

func atMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func msAt(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// searchError keeps a refused query a refusal: a bad expression or an
// impossible window is the caller's mistake, not the daemon's failure.
func searchError(err error) error {
	if _, ok := status.FromError(err); ok && status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.InvalidArgument, err.Error())
}
