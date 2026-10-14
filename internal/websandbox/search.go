package websandbox

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type searchCursor struct {
	Session    string
	Query      string
	IgnoreCase bool
	Snapshot   uint64
	Before     uint64
}

// Simulate the session-scoped substring search without a Manager or disk index.
// The cursor fences the journal so live responses cannot shift older pages.
func (x *Sessions) Search(r *resource.SessionSearchRequest, stream grpc.ServerStreamingServer[resource.SessionSearchReply]) error {
	if len(r.GetSessions()) != 1 || (r.GetMatch() != "" && r.GetMatch() != "substring") {
		return status.Error(codes.Unimplemented, "sandbox search supports one session and substring matching")
	}
	cursor := searchCursor{Session: r.GetSessions()[0], Query: r.GetQuery(), IgnoreCase: r.GetIgnoreCase()}
	if raw := r.GetCursor(); raw != "" {
		if len(raw) > 4096 {
			return status.Error(codes.InvalidArgument, "invalid search cursor")
		}
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || json.Unmarshal(b, &cursor) != nil || cursor.Session != r.GetSessions()[0] || cursor.Query != r.GetQuery() || cursor.IgnoreCase != r.GetIgnoreCase() {
			return status.Error(codes.InvalidArgument, "search cursor does not match the query")
		}
	}
	x.S.mu.Lock()
	st, err := x.S.find(resource.SessionRef_builder{RuntimeId: proto.String(cursor.Session)}.Build())
	if err != nil {
		x.S.mu.Unlock()
		return err
	}
	if r.GetCursor() == "" && len(st.events) > 0 {
		cursor.Snapshot = st.events[len(st.events)-1].GetSeq()
	}
	limit := min(500, max(1, int(r.GetLimit())))
	if r.GetLimit() == 0 {
		limit = 100
	}
	snippet := min(2048, max(1, int(r.GetSnippet())))
	if r.GetSnippet() == 0 {
		snippet = 240
	}
	query := cursor.Query
	if cursor.IgnoreCase {
		query = strings.ToLower(query)
	}
	hits := make([]*resource.SessionSearchHit, 0, limit)
	more := false
	for _, event := range slices.Backward(st.events) {
		if err := stream.Context().Err(); err != nil {
			x.S.mu.Unlock()
			return err
		}
		if event.GetSeq() > cursor.Snapshot || (cursor.Before > 0 && event.GetSeq() >= cursor.Before) {
			continue
		}
		if event.GetKind() != "input" && event.GetKind() != "assistant" {
			continue
		}
		text := event.GetText()
		haystack := text
		if cursor.IgnoreCase {
			haystack = strings.ToLower(haystack)
		}
		at := strings.Index(haystack, query)
		if at < 0 {
			continue
		}
		if len(hits) == limit {
			more = true
			break
		}
		start := max(0, utf8.RuneCountInString(text[:min(at, len(text))])-snippet/4)
		runes := []rune(text)
		excerpt := string(runes[start:min(len(runes), start+snippet)])
		hits = append(hits, resource.SessionSearchHit_builder{Seq: proto.Uint64(event.GetSeq()), TimeMs: proto.Int64(event.GetTimeMs()), Kind: proto.String(event.GetKind()), Bytes: proto.Int32(int32(len(text))), Snippet: proto.String(excerpt)}.Build())
	}
	visit := resource.SessionSearchVisit_builder{SessionId: proto.String(st.value.GetRuntimeId()), Alias: proto.String(st.value.GetAlias()), Title: proto.String(st.value.GetName()), Agent: proto.String(st.value.GetAgent()), ProjectId: proto.String(st.value.GetProject().GetRuntimeId()), Hits: hits}.Build()
	x.S.mu.Unlock()
	next := ""
	if more {
		cursor.Before = hits[len(hits)-1].GetSeq()
		b, _ := json.Marshal(cursor)
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	if len(hits) > 0 {
		if err = stream.Send(resource.SessionSearchReply_builder{Visit: visit}.Build()); err != nil {
			return err
		}
	}
	return stream.Send(resource.SessionSearchReply_builder{Summary: resource.SessionSearchSummary_builder{Sessions: proto.Int32(1), Hits: proto.Int32(int32(len(hits))), HasMore: proto.Bool(more), NextCursor: proto.String(next)}.Build()}.Build())
}
