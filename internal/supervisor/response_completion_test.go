package supervisor

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"testing"
	"time"
)

func completion(t *testing.T, s *Supervisor) core.ResponseCompletion {
	t.Helper()
	for _, e := range s.log.All() {
		if e.Kind == "turn_end" && e.Response != nil && e.Response.CompletionJSON != "" {
			var c core.ResponseCompletion
			if err := json.Unmarshal([]byte(e.Response.CompletionJSON), &c); err != nil {
				t.Fatal(err)
			}
			return c
		}
	}
	t.Fatal("missing final response summary")
	return core.ResponseCompletion{}
}

func TestClaudeFinalResponseMetrics(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "input", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}`))
	s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"answer"}]}}`))
	s.consume([]byte(`{"type":"assistant","parent_tool_use_id":"child","message":{"content":[{"type":"text","text":"child answer"}]}}`))
	s.consume([]byte(`{"type":"result","subtype":"success","duration_ms":4250,"duration_api_ms":3000,"total_cost_usd":0.02,"num_turns":2,"usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}`))
	c := completion(t, s)
	responses := assistantEvents(s)
	if c.ResponseSeq != responses[1].Seq || c.FinalSource != "turn_end" || *c.DurationMS != 4250 || c.DurationSource != "provider" || c.Metrics["cache_read_tokens"] != 20 || c.Metrics["output_tokens"] != 40 || c.Metrics["cost_usd"] != 0.02 || c.TokenScope != "turn" {
		t.Fatal(c)
	}
	for _, e := range responses {
		if e.TimeMS <= 0 {
			t.Fatal("intermediate timestamp missing")
		}
	}
}

func TestCodexFinalPhaseAndMetricsScope(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "input", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"t"}}}`))
	s.consume([]byte(`{"method":"item/completed","params":{"turnId":"t","item":{"type":"agentMessage","id":"a","phase":"commentary","text":"working"}}}`))
	s.consume([]byte(`{"method":"item/completed","params":{"turnId":"t","item":{"type":"agentMessage","id":"b","phase":"final_answer","text":"done"}}}`))
	s.consume([]byte(`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread","turnId":"t","tokenUsage":{"total":{"totalTokens":999999},"last":{"inputTokens":100,"outputTokens":50,"totalTokens":150,"cachedInputTokens":30,"reasoningOutputTokens":10}}}}`))
	s.consume([]byte(`{"method":"thread/tokenUsage/updated","params":{"turnId":"other","tokenUsage":{"last":{"totalTokens":888}}}}`))
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"t","status":"completed","durationMs":1800}}}`))
	c := completion(t, s)
	r := assistantEvents(s)
	if r[0].Response.Phase != "commentary" || r[1].Response.Phase != "final_answer" || c.ResponseSeq != r[1].Seq || c.FinalSource != "phase" || *c.DurationMS != 1800 || c.DurationSource != "provider" || c.TokenScope != "last_call" || c.Metrics["total_tokens"] != 150 || c.Metrics["reasoning_tokens"] != 10 {
		t.Fatal(c)
	}
}

func TestResponseElapsedFallbackAndUnknownFinal(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.responseTracking.started = time.Now().Add(-time.Second)
	s.consume([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"answer"}]}}`))
	s.consume([]byte(`{"type":"result","subtype":"success"}`))
	c := completion(t, s)
	if c.DurationSource != "cxz" || c.DurationMS == nil || *c.DurationMS < 1000 || *c.DurationMS > 2000 {
		t.Fatal(c)
	}
	for _, status := range []string{"completed", "failed", "interrupted"} {
		s.responseTracking = responseTracking{last: 42, phase: "commentary"}
		if s.completeResponse(status, nil).CompletionJSON != "" {
			t.Fatal("commentary promoted to final")
		}
	}
}
