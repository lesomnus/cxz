package supervisor

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

const codexProgressTimeout = 2 * time.Minute
const codexInterruptGrace = 15 * time.Second

type codexProgress struct {
	last      time.Time
	interrupt time.Time
	items     map[string]bool
}

// Telemetry polling is not turn progress. Native running items and human
// approvals are excluded from the response watchdog; their outcomes are not
// inferred from silence, and no user input or tool call is replayed.
func (c *codexProtocol) observeProgress(method string, params []byte, now time.Time) {
	if method == "turn/started" || method == "turn/completed" {
		c.progress = codexProgress{last: now}
		return
	}
	if method == "serverRequest/resolved" {
		c.progress.last = now
		return
	}
	if !strings.HasPrefix(method, "item/") {
		return
	}
	c.progress.last = now
	var p struct {
		Item struct{ ID, Type string }
	}
	if json.Unmarshal(params, &p) != nil || p.Item.ID == "" {
		return
	}
	if method == "item/completed" {
		delete(c.progress.items, p.Item.ID)
	} else if method == "item/started" && p.Item.Type != "agentMessage" && p.Item.Type != "reasoning" && p.Item.Type != "userMessage" {
		if c.progress.items == nil {
			c.progress.items = map[string]bool{}
		}
		c.progress.items[p.Item.ID] = true
	}
}

func (c *codexProtocol) checkProgress(now time.Time) {
	s := c.s
	if c.turn == "" || s.snap.State != "working" || s.stopping || s.hasBlockingPending() {
		// Resuming after human input must get a fresh response budget.
		c.progress.last = now
		return
	}
	if !c.progress.interrupt.IsZero() {
		if now.Sub(c.progress.interrupt) >= codexInterruptGrace {
			s.event("diagnostic", "Codex did not acknowledge the response-timeout interrupt; resume the session to reconnect. Saved history is preserved.", "", nil, nil)
			s.event("state", "failed", "", nil, nil)
			s.kill()
		}
		return
	}
	if c.progress.last.IsZero() || len(c.progress.items) > 0 {
		c.progress.last = now
		return
	}
	limit := codexProgressTimeout
	if v, err := time.ParseDuration(os.Getenv("CXZ_CODEX_PROGRESS_TIMEOUT")); err == nil && v > 0 {
		limit = v
	}
	if now.Sub(c.progress.last) < limit {
		return
	}
	c.progress.interrupt = now
	s.interrupted = true
	s.event("diagnostic", "Codex response progress timed out; interrupting the turn. Saved history is preserved and no commands are retried.", "", nil, nil)
	if err := s.write(rpc("cxz-progress-interrupt", "turn/interrupt", map[string]any{"threadId": s.snap.VendorID, "turnId": c.turn})); err != nil {
		s.event("diagnostic", "Codex timeout interrupt could not be delivered; resume the session to reconnect.", "", nil, nil)
		s.event("state", "failed", "", nil, nil)
		s.kill()
	}
}
