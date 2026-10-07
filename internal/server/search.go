package server

import (
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Search answers for every conversation the installation holds.
//
// There are two installations to answer for, and the difference is where the
// journals are. A manager keeps each project's in that project's own storage,
// so it fans out; a host-local cxz has one state directory and reads it
// directly. The stream is the same either way, because the caller's question
// was the same.
func (s *Server) Search(r *api.SearchRequest, stream grpc.ServerStreamingServer[api.SearchReply]) error {
	ctx := stream.Context()
	q := conversation.ScanQuery{
		Query:        r.Query,
		Match:        r.Match,
		IgnoreCase:   r.IgnoreCase,
		View:         r.View,
		IncludeTools: r.IncludeTools,
		Since:        conversation.MSTime(r.SinceMs),
		Until:        conversation.MSTime(r.UntilMs),
		Sessions:     r.Sessions,
		Snippet:      int(r.Snippet),
		Limit:        int(r.Limit),
	}
	if err := s.requireManagerAuthority(ctx, "search conversations from the host client, not from inside a project"); err != nil {
		return err
	}
	if s.manager != nil {
		out, err := s.manager.Search(ctx, workspace.SearchRequest{Query: q, Projects: r.Projects, Exclude: r.Exclude, Cursor: r.Cursor}, func(v workspace.SearchVisit) error {
			return stream.Send(&api.SearchReply{Visit: searchVisit(v)})
		}, func(p workspace.SearchProgress) error {
			return stream.Send(&api.SearchReply{Progress: &api.SearchProgress{ProjectId: p.ProjectID, ProjectName: p.ProjectName, State: p.State, Message: p.Message, Opened: int32(p.Opened), Total: int32(p.Total)}})
		})
		if err != nil {
			return searchError(err)
		}
		return stream.Send(&api.SearchReply{Summary: &api.SearchSummary{
			Projects: int32(out.Projects), Unavailable: int32(out.Unavailable), Sessions: int32(out.Sessions),
			Hits: int32(out.Hits), Truncated: int32(out.Truncated), NextCursor: out.NextCursor, HasMore: out.HasMore,
			SinceMs: conversation.TimeMS(out.Since), UntilMs: conversation.TimeMS(out.Until),
		}})
	}
	cursor := conversation.ScanCursor{Version: 1}
	if r.Cursor != "" {
		var err error
		if cursor, err = conversation.DecodeScanCursor(r.Cursor); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		q.Since, q.Until = cursor.Window()
		q.Resume = cursor.At[""]
	}
	if q.Until.IsZero() {
		q.Until = time.Now().UTC()
	}
	scanner := &conversation.Scanner{Root: s.root}
	found := int32(0)
	out, err := scanner.Scan(ctx, q, func(v conversation.ScanVisit) error {
		if len(v.Hits) == 0 {
			return nil
		}
		found++
		return stream.Send(&api.SearchReply{Visit: searchVisit(workspace.SearchVisit{ScanVisit: v})})
	})
	if err != nil {
		return searchError(err)
	}
	// Sessions counts the conversations a person was shown, not the ones that
	// were read: "two conversations" beside one hit reads as a miscount.
	summary := &api.SearchSummary{Projects: 1, Sessions: found, Hits: int32(out.Hits), Truncated: int32(out.Truncated),
		SinceMs: conversation.TimeMS(q.Since), UntilMs: conversation.TimeMS(q.Until)}
	if out.Next != nil {
		cursor.SinceMS, cursor.UntilMS = conversation.TimeMS(q.Since), conversation.TimeMS(q.Until)
		cursor.At = map[string]conversation.Resume{"": *out.Next}
		summary.HasMore = true
		summary.NextCursor = cursor.Encode()
	}
	return stream.Send(&api.SearchReply{Summary: summary})
}

func searchVisit(v workspace.SearchVisit) *api.SearchVisit {
	out := &api.SearchVisit{
		ProjectId: v.ProjectID, ProjectName: v.ProjectName,
		SessionId: v.ID, Alias: v.Alias, Title: v.Title, Agent: v.Agent, State: v.State,
		ActivityMs: conversation.TimeMS(v.Activity), CreatedMs: conversation.TimeMS(v.CreatedAt),
		Truncated: v.Truncated,
	}
	for _, h := range v.Hits {
		out.Hits = append(out.Hits, &api.SearchHit{Seq: h.Seq, TimeMs: conversation.TimeMS(h.Time), Kind: h.Kind, Bytes: int32(h.Bytes), Score: int32(h.Score), Snippet: h.Snippet})
	}
	return out
}

// searchError keeps a refused query a refusal: a bad expression or an
// impossible window is the caller's mistake, not the daemon's failure.
func searchError(err error) error {
	if _, ok := status.FromError(err); ok && status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.InvalidArgument, err.Error())
}
