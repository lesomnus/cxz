package lifecycle

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s SessionServer) ConversationStats(ctx context.Context, r *resource.ConversationStatsRequest) (*resource.ConversationStatsReply, error) {
	now := time.Now().UnixMilli()
	to := r.GetToMs()
	if to == 0 {
		to = now
	}
	from := r.GetFromMs()
	if from == 0 {
		from = to - int64(30*24*time.Hour/time.Millisecond)
	}
	if from < 0 || to <= from || to-from > int64(366*24*time.Hour/time.Millisecond) || r.HasProject() && r.HasSession() {
		return nil, status.Error(codes.InvalidArgument, "provide at most one scope and a positive time range of at most 366 days")
	}
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	var sessions []*resource.Session
	if r.HasSession() {
		v, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetSession(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			return nil, err
		}
		if !v.GetListed() {
			return nil, status.Error(codes.NotFound, "session deleted")
		}
		sessions = append(sessions, v)
	} else {
		filter := resource.SessionFilter_builder{Listed: ptr(true)}.Build()
		if r.HasProject() {
			p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			if !p.GetListed() {
				return nil, status.Error(codes.NotFound, "project deleted")
			}
			filter.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
		}
		after := ""
		for {
			page, err := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{filter}, Size: 200, After: after}.Build())
			if err != nil {
				return nil, err
			}
			sessions = append(sessions, page.GetItems()...)
			if len(sessions) > 1000 {
				return nil, status.Error(codes.ResourceExhausted, "statistics scope exceeds 1000 sessions")
			}
			if page.GetNext() == "" {
				break
			}
			if page.GetNext() == after {
				return nil, status.Error(codes.Internal, "session cursor did not advance")
			}
			after = page.GetNext()
		}
	}
	total := newStatsValues()
	days := map[string]*resource.ConversationStatsValues{}
	result := resource.ConversationStatsReply_builder{FromMs: ptr(from), ToMs: ptr(to), GeneratedMs: ptr(now), Total: total, Complete: ptr(true)}.Build()
	var coverage []*resource.ConversationStatsCoverage
	var count, bytes uint64
	for _, session := range sessions {
		c := resource.ConversationStatsCoverage_builder{SessionId: session.GetId(), Alias: ptr(session.GetAlias()), SnapshotSeq: ptr(session.GetStatus().GetLastSeq()), Complete: ptr(true)}.Build()
		state := statsUsage{cost: map[string]float64{}, known: map[string]bool{}}
		var after uint64
		for after < c.GetSnapshotSeq() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			batch, err := s.shared.runtime.History(ctx, &api.WatchRequest{SessionId: session.GetRuntimeId(), AfterSeq: after})
			if err != nil {
				return nil, err
			}
			previous := after
			for _, e := range batch.Events {
				if e.Seq > c.GetSnapshotSeq() {
					break
				}
				if e.Seq <= after {
					return nil, status.Error(codes.Internal, "history cursor did not advance")
				}
				if e.Seq != after+1 {
					c.SetComplete(false)
					state = statsUsage{cost: map[string]float64{}, known: map[string]bool{}, gap: true}
				}
				after = e.Seq
				if c.GetFirstSeq() == 0 {
					c.SetFirstSeq(after)
				}
				c.SetLastSeq(after)
				count++
				bytes += uint64(len(e.Text) + len(e.Payload))
				if count > 100000 || bytes > 64<<20 {
					return nil, status.Error(codes.ResourceExhausted, "statistics scan exceeds 100000 events or 64 MiB; narrow the scope")
				}
				v := state.event(e)
				if e.TimeMs < from || e.TimeMs >= to {
					continue
				}
				date := time.UnixMilli(e.TimeMs).UTC().Format("2006-01-02")
				if days[date] == nil {
					days[date] = newStatsValues()
				}
				addStats(total, v)
				addStats(days[date], v)
			}
			if after == previous {
				break
			}
		}
		if after < c.GetSnapshotSeq() {
			c.SetComplete(false)
		}
		if !c.GetComplete() {
			result.SetComplete(false)
		}
		coverage = append(coverage, c)
	}
	result.SetSessions(coverage)
	dates := make([]string, 0, len(days))
	for date := range days {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	for _, date := range dates {
		result.SetDays(append(result.GetDays(), resource.ConversationStatsDay_builder{Date: ptr(date), Values: days[date]}.Build()))
	}
	return result, nil
}

func newStatsValues() *resource.ConversationStatsValues {
	return resource.ConversationStatsValues_builder{}.Build()
}

// Input tokens exclude cache reads/writes; normalize Codex's inclusive input count.
type statsUsage struct {
	pending map[string]any
	run     string
	cost    map[string]float64
	known   map[string]bool
	gap     bool
}

func object(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
func statsNumber(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		v, ok := m[key].(float64)
		if ok && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) && v < math.MaxInt64 {
			return v, true
		}
	}
	return 0, false
}
func (s *statsUsage) event(e *api.Event) *resource.ConversationStatsValues {
	v := newStatsValues()
	v.SetEvents(1)
	v.SetEventContentBytes(uint64(len(e.Text) + len(e.Payload)))
	if e.Kind == "input" {
		v.SetInputMessages(1)
		s.pending = nil
	}
	if e.Kind == "assistant" {
		v.SetAssistantEvents(1)
	}
	if e.Kind == "input" || e.Kind == "assistant" {
		v.SetConversationBytes(uint64(len(e.Text)))
	}
	var root map[string]any
	_ = json.Unmarshal(e.Payload, &root)
	if e.Kind == "usage" && e.Text == "thread/tokenUsage/updated" {
		s.pending = object(object(root, "tokenUsage"), "last")
		s.run = e.RunId
	}
	if e.Kind != "turn_end" {
		return v
	}
	v.SetFinishedTurns(1)
	u := object(root, "usage")
	if len(u) == 0 {
		u = object(object(root, "turn"), "usage")
	}
	if len(u) == 0 && s.run == e.RunId {
		u = s.pending
	}
	s.pending = nil
	reported := false
	for _, field := range []struct {
		keys []string
		set  func(uint64)
	}{
		{[]string{"input_tokens", "inputTokens"}, v.SetInputTokens},
		{[]string{"output_tokens", "outputTokens"}, v.SetOutputTokens},
		{[]string{"cache_read_input_tokens", "cachedInputTokens"}, v.SetCacheReadTokens},
		{[]string{"cache_creation_input_tokens"}, v.SetCacheWriteTokens},
	} {
		if n, ok := statsNumber(u, field.keys...); ok {
			field.set(uint64(n))
			reported = true
		}
	}
	if _, codex := u["inputTokens"]; codex {
		v.SetInputTokens(v.GetInputTokens() - min(v.GetInputTokens(), v.GetCacheReadTokens()))
	}
	if reported {
		v.SetTokenReportedTurns(1)
	}
	if n, ok := statsNumber(root, "total_cost_usd"); ok {
		if s.known[e.RunId] || !s.gap {
			delta := n - s.cost[e.RunId]
			if delta < 0 {
				delta = n
			}
			v.SetReportedCostUsd(delta)
			v.SetCostReportedTurns(1)
		}
		s.cost[e.RunId] = n
		s.known[e.RunId] = true
	} else if n, ok := statsNumber(root, "costUSD", "costUsd"); ok {
		v.SetReportedCostUsd(n)
		v.SetCostReportedTurns(1)
	}
	return v
}
func addStats(a, b *resource.ConversationStatsValues) {
	a.SetEvents(a.GetEvents() + b.GetEvents())
	a.SetInputMessages(a.GetInputMessages() + b.GetInputMessages())
	a.SetAssistantEvents(a.GetAssistantEvents() + b.GetAssistantEvents())
	a.SetConversationBytes(a.GetConversationBytes() + b.GetConversationBytes())
	a.SetEventContentBytes(a.GetEventContentBytes() + b.GetEventContentBytes())
	a.SetFinishedTurns(a.GetFinishedTurns() + b.GetFinishedTurns())
	a.SetInputTokens(a.GetInputTokens() + b.GetInputTokens())
	a.SetOutputTokens(a.GetOutputTokens() + b.GetOutputTokens())
	a.SetCacheReadTokens(a.GetCacheReadTokens() + b.GetCacheReadTokens())
	a.SetCacheWriteTokens(a.GetCacheWriteTokens() + b.GetCacheWriteTokens())
	a.SetTokenReportedTurns(a.GetTokenReportedTurns() + b.GetTokenReportedTurns())
	a.SetReportedCostUsd(a.GetReportedCostUsd() + b.GetReportedCostUsd())
	a.SetCostReportedTurns(a.GetCostReportedTurns() + b.GetCostReportedTurns())
}
