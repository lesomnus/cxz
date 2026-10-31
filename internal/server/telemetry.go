package server

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

// TelemetryStats is optional at the resource boundary. Standalone runtimes and
// installations with telemetry disabled retain the bounded history scan.
func (s *Server) TelemetryStats(ctx context.Context, scope, id string, from, to int64) (*resource.ConversationStatsReply, bool, error) {
	if s.manager == nil {
		return nil, false, nil
	}
	return s.manager.TelemetryStats(ctx, scope, id, from, to)
}
