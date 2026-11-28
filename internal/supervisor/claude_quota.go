package supervisor

import (
	"encoding/json"
	"time"
)

func (s *Supervisor) recordClaudeQuota(raw json.RawMessage, observed int64) {
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil || data == nil {
		return
	}
	data["_cxz_observed_ms"], _ = json.Marshal(observed)
	// Not folded into the latest value the way Codex's buckets are: claude's
	// payload answers differently almost every time. Measured on 12,301 of
	// these events, 12,293 payloads were distinct, 9,743 were still distinct
	// without the observation stamp above, and 9,662 differed in `rate_limits`
	// alone -- the only field anything renders. The 36MiB this costs is the
	// unread remainder of each payload, not repetition, so it is a question of
	// what a usage event should carry rather than of how often. See cxz#336.
	s.event("usage", "get_usage", "", data, nil)
}

func (s *Supervisor) publishClaudeQuota(raw json.RawMessage) {
	s.recordClaudeQuota(raw, time.Now().UnixMilli())
}
