package lifecycle

import (
	"context"
	"encoding/hex"
	"sort"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/usagemetrics"
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
	if from < 0 || to <= from || to-from > int64(366*24*time.Hour/time.Millisecond) || boolCount(r.HasProject(), r.HasSession(), r.HasAccount()) > 1 {
		return nil, status.Error(codes.InvalidArgument, "provide at most one scope and a positive time range of at most 366 days")
	}
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	if analytics, ok := s.shared.runtime.(interface {
		TelemetryStats(context.Context, string, string, int64, int64) (*resource.ConversationStatsReply, bool, error)
	}); ok {
		scope, id := "", ""
		if r.HasSession() {
			v, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetSession(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			scope, id = "session", hex.EncodeToString(v.GetId())
		} else if r.HasProject() {
			v, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			scope, id = "project", hex.EncodeToString(v.GetId())
		} else if r.HasAccount() {
			v, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: r.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			scope, id = "account", hex.EncodeToString(v.GetId())
		}
		result, enabled, err := analytics.TelemetryStats(ctx, scope, id, from, to)
		if enabled {
			return result, err
		}
	}
	var sessions []*resource.Session
	if r.HasSession() {
		v, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetSession(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, v)
	} else {
		filter := resource.SessionFilter_builder{}.Build()
		if r.HasAccount() {
			account, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: r.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			filter.SetAccount(resource.AccountRef_builder{Id: account.GetId()}.Build())
		}
		if r.HasProject() {
			p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			filter.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
		}
		var filters []*resource.SessionFilter
		if r.HasAccount() || r.HasProject() {
			filters = []*resource.SessionFilter{filter}
		}
		after := ""
		for {
			page, err := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: filters, Size: 200, After: after}.Build())
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
		state := statsUsage{}
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
					state = statsUsage{State: usagemetrics.State{Gap: true}}
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

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func newStatsValues() *resource.ConversationStatsValues {
	return resource.ConversationStatsValues_builder{}.Build()
}

type statsUsage struct{ usagemetrics.State }

func (s *statsUsage) event(e *api.Event) *resource.ConversationStatsValues {
	u := s.Event(core.Event{Kind: e.Kind, Text: e.Text, Payload: e.Payload, RunID: e.RunId})
	v := newStatsValues()
	v.SetEvents(u.Events)
	v.SetInputMessages(u.InputMessages)
	v.SetAssistantEvents(u.AssistantEvents)
	v.SetConversationBytes(u.ConversationBytes)
	v.SetEventContentBytes(u.EventContentBytes)
	v.SetFinishedTurns(u.FinishedTurns)
	v.SetInputTokens(u.InputTokens)
	v.SetOutputTokens(u.OutputTokens)
	v.SetCacheReadTokens(u.CacheReadTokens)
	v.SetCacheWriteTokens(u.CacheWriteTokens)
	v.SetTokenReportedTurns(u.TokenReportedTurns)
	v.SetCostReportedTurns(u.CostReportedTurns)
	v.SetReportedCostUsd(u.ReportedCostUSD)
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
