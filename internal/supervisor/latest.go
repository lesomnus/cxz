package supervisor

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/lesomnus/cxz/internal/agentview"
)

// A polled state -- the model catalog, an account's remaining quota, a
// provider's availability -- is not a thing that happened. It is the present
// value of something, asked for again every minute because that is how a
// provider reports it, and recorded as an event because that is the only shape
// a session had for it.
//
// Measured on this installation (see cxz#344): one session held 12,284
// `usage:get_usage` events for 36.3MiB and 12,368 `models:catalog` events for
// 21.9MiB -- 62% of its 100MiB budget -- while its conversation text was 4KiB.
// Across five sessions the catalog was 33,416 events carrying 13 distinct
// values, and 75-92% of every session's history had already been deleted to
// make room for it.
//
// So a poll that answers what the conversation already says records nothing.
// What it records instead, where a reader needs to know the value is still
// being watched, is that it was observed -- which is small and does not carry
// the value again.

func latestKey(kind, text string) string { return kind + "\x00" + text }

// publishQuota records an account's remaining quota, or that the quota it
// already recorded was seen again.
//
// Codex answers the same buckets for hours: 6,321 of these events carried 190
// distinct values, and 4,647 carried 141. A reader renders the latest value, so
// the repetitions say nothing about the account -- but they do say the value is
// current, and a reader that cannot tell a value from a stale one marks it
// approximate (internal/tui/quota.go). The marker is what carries that, at
// about a tenth of the payload's size.
func (s *Supervisor) publishQuota(text string, payload json.RawMessage) {
	if s.latestUnchanged(latestKey("usage", text), payload) {
		s.event("usage_status", agentview.QuotaObserved, "", nil, nil)
		return
	}
	s.event("usage", text, "", payload, nil)
}

// latestUnchanged answers whether this is the value already recorded under
// key, and remembers it when it is not.
//
// The comparison is on the meaning, not the bytes. cxz stamps a quota payload
// with the instant it was read (`_cxz_observed_ms`, quota_shared.go), so no two
// are ever equal as bytes: the measurement found 12,276 distinct payloads in
// 12,284 events. A caller either compares before it stamps, or names the
// non-semantic fields here.
//
// A payload that cannot be read as an object is compared whole, and a marshal
// that fails is reported as changed -- the fallback in both directions is to
// record the event, which is what this code did before.
func (s *Supervisor) latestUnchanged(key string, payload any, omit ...string) bool {
	b, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if len(omit) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) == nil && fields != nil {
			for _, name := range omit {
				delete(fields, name)
			}
			if reduced, err := json.Marshal(fields); err == nil {
				b = reduced
			}
		}
	}
	sum := sha256.Sum256(b)
	digest := string(sum[:])
	if s.latest[key] == digest {
		return true
	}
	if s.latest == nil {
		s.latest = map[string]string{}
	}
	s.latest[key] = digest
	return false
}

// forgetLatest makes the next poll record its value again.
//
// Retention keeps a conversation's prefix as a checkpoint of what a reader
// needs to resume, and the polled states are not in it: `makeHistoryCheckpoint`
// carries settings, receipts and background work, so a trim takes the last
// catalog and the last quota with it. Republishing every minute is what used to
// put them back. Nothing else does, so a trim has to ask for them again.
func (s *Supervisor) forgetLatest() { s.latest = nil }
