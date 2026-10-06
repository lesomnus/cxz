package websandbox

import (
	"encoding/json"
	"github.com/lesomnus/cxz/resource"
	"strconv"
)

// These values are simulated, as is all provider output in the design sandbox.
func (s *Server) complete(st *session, seq uint64) {
	metrics := map[string]float64{"input_tokens": 1200, "output_tokens": 320, "cache_read_tokens": 800, "tool_calls": 3, "cost_usd": 0.0123}
	scope := "turn"
	if st.value.GetAgent() == "codex" {
		delete(metrics, "cost_usd")
		scope = "last_call"
		metrics["reasoning_tokens"] = 120
	}
	b, _ := json.Marshal(map[string]any{"response_seq": strconv.FormatUint(seq, 10), "final_source": "turn_end", "duration_ms": 4200, "duration_source": "provider", "token_scope": scope, "metrics": metrics})
	s.event(st, "turn_end", "completed", "", nil)
	st.events[len(st.events)-1].SetResponse(resource.ResponseMetadata_builder{CompletionJson: b}.Build())
}
