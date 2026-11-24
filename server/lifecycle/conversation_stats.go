package lifecycle

import (
	"context"
	"encoding/hex"
	"time"

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
	// Usage is collected by the managed telemetry store and nowhere else. When
	// it cannot answer -- a Manager without the store, one still bringing it
	// up, or one whose store failed -- usage is unavailable, not recomputed from
	// conversation history: a second path would disagree with the first about
	// what an account spent. See docs/managed-telemetry.md.
	analytics, ok := s.shared.runtime.(interface {
		TelemetryStats(context.Context, string, string, int64, int64) (*resource.ConversationStatsReply, bool, error)
	})
	if !ok {
		return nil, unavailableStats()
	}
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
	if !enabled {
		return nil, unavailableStats()
	}
	return result, err
}

func unavailableStats() error {
	return status.Error(codes.Unavailable, "usage telemetry is unavailable on this server; retry once its telemetry store is running")
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
