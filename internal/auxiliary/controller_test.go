package auxiliary

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func profile() Profile {
	return Profile{Enabled: true, Account: "work", Agent: "codex", Backend: accounts.BrokeredAccessToken, Model: "test", Effort: "low"}
}
func events(id string, n uint64, user, answer string) []*api.Event {
	return []*api.Event{{TimeMs: time.Now().UnixMilli() + 1, SessionId: id, RunId: "run", Seq: n, Kind: "input", Text: user}, {TimeMs: time.Now().UnixMilli() + 1, SessionId: id, RunId: "run", Seq: n + 1, Kind: "assistant", Text: answer}, {TimeMs: time.Now().UnixMilli() + 1, SessionId: id, RunId: "run", Seq: n + 2, Kind: "turn_end", Text: "completed"}}
}
func waitJob(t *testing.T, c *Controller, id, status string) *Job {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		j, e := c.Status(id)
		if e != nil {
			t.Fatal(e)
		}
		if j != nil && j.Status == status {
			return j
		}
		time.Sleep(time.Millisecond)
	}
	j, _ := c.Status(id)
	t.Fatalf("wanted %s: %+v", status, j)
	return nil
}
func TestDeduplicateCombineAndIsolate(t *testing.T) {
	var calls atomic.Int32
	c, e := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		calls.Add(1)
		if in.Task != "combined" {
			t.Errorf("not combined: %s", in.Task)
		}
		if strings.Contains(in.Text, "secret A") && strings.Contains(in.Text, "secret B") {
			t.Error("cross-session context")
		}
		return Output{Summary: "done", Suggestion: "next", Usage: json.RawMessage(`{"input_tokens":4}`)}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c.Observe(events("off", 1, "ignored", "ignored"))
	if j, _ := c.Status("off"); j != nil {
		t.Fatal("disabled generated")
	}
	defer c.Close()
	_, _ = c.Save("summary", profile())
	_, _ = c.Save("suggestion", profile())
	batch := events("a", 1, "secret A", "answer A")
	c.Observe(batch)
	j := waitJob(t, c, "a", "completed")
	if j.Suggestion != "next" || len(j.Usage) != 1 {
		t.Fatalf("%+v", j)
	}
	c.Observe(batch)
	c.Observe(events("b", 1, "secret B", "answer B"))
	waitJob(t, c, "b", "completed")
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
}
func TestCancelOnNewInputAndConfigRevision(t *testing.T) {
	started := make(chan struct{}, 2)
	c, _ := New(t.TempDir(), func(ctx context.Context, _ Input) (Output, error) {
		started <- struct{}{}
		<-ctx.Done()
		return Output{}, ctx.Err()
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	c.Observe(events("a", 1, "first", "answer"))
	<-started
	c.Observe([]*api.Event{{TimeMs: time.Now().UnixMilli() + 1, SessionId: "a", RunId: "run", Seq: 4, Kind: "input", Text: "changed"}})
	waitJob(t, c, "a", "stale")
	c.Observe(events("b", 1, "other", "answer"))
	<-started
	_, _ = c.Save("summary", Profile{})
	waitJob(t, c, "b", "stale")
}
func TestCheckpointOnlyOnOverflowAndFailurePreservesState(t *testing.T) {
	var checkpoints atomic.Int32
	fail := atomic.Bool{}
	c, _ := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		if len(in.Text) > MaxInput {
			t.Error("input overflow")
		}
		if in.Task == "checkpoint" {
			checkpoints.Add(1)
			if fail.Load() {
				return Output{}, errors.New("offline")
			}
			return Output{Checkpoint: "user constraints; planned not done"}, nil
		}
		return Output{Summary: "summary"}, nil
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	c.Observe(events("a", 1, "small", "reply"))
	waitJob(t, c, "a", "completed")
	if checkpoints.Load() != 0 {
		t.Fatal("compacted early")
	}
	c.Observe(events("a", 4, strings.Repeat("a", 7000), strings.Repeat("b", 7000)))
	waitJob(t, c, "a", "completed")
	c.Observe(events("a", 7, strings.Repeat("c", 7000), strings.Repeat("d", 7000)))
	waitJob(t, c, "a", "completed")
	if checkpoints.Load() != 1 {
		t.Fatal(checkpoints.Load())
	}
	fail.Store(true)
	c.Observe(events("a", 10, strings.Repeat("e", 7000), strings.Repeat("f", 7000)))
	waitJob(t, c, "a", "failed")
	c.mu.Lock()
	s, e := c.load("a")
	c.mu.Unlock()
	if e != nil || s.Checkpoint != "user constraints; planned not done" {
		t.Fatal(s, e)
	}
}
func TestRestartDoesNotReplayAmbiguousCall(t *testing.T) {
	root := t.TempDir()
	c, _ := New(root, nil)
	c.mu.Lock()
	_ = c.save("s", State{Job: &Job{ID: "id", Status: "running"}})
	c.mu.Unlock()
	next, _ := New(root, func(context.Context, Input) (Output, error) { t.Error("replayed"); return Output{}, nil })
	waitJob(t, next, "s", "interrupted")
}
func TestBoundedUTF8AndValidation(t *testing.T) {
	s := Clip(strings.Repeat("한글", 1000), 1001)
	if !utf8.ValidString(s) || len(s) > 1001 {
		t.Fatal("invalid clip")
	}
	if e := ValidateModel(profile(), []agentview.ModelOption{{ID: "test", Efforts: []string{"high"}}}); e == nil {
		t.Fatal("accepted unsupported effort")
	}
	if e := ValidateModel(profile(), []agentview.ModelOption{{ID: "test", Efforts: []string{"low"}}}); e != nil {
		t.Fatal(e)
	}
	if _, e := decodeOutput(`{"summary":"ok","suggestion":"","checkpoint":"","usage":{"cost":999}}`); e != nil {
		t.Fatal(e)
	}
	// Overshooting a prompt budget many times over is still a reply, not an
	// error: only the whole reply passing MaxOutput is refused, and only because
	// half a JSON object cannot be read.
	long := `{"summary":"` + strings.Repeat("x", SummaryBudget*20) + `","suggestion":"","checkpoint":""}`
	if _, e := decodeOutput(long); e != nil {
		t.Fatal("decoding refused a long field instead of leaving its limit to the consumer:", e)
	}
	if _, e := decodeOutput(`{"summary":"` + strings.Repeat("x", MaxOutput) + `"}`); e == nil {
		t.Fatal("a reply past MaxOutput was accepted")
	}
}

// A reply that overshoots a field limit used to be discarded whole and reported
// as a failure, even when the oversized field was one the task never reads. What
// arrived is shown instead, clipped, with the truncation marked.
func TestOverlongReplyIsClippedNotDiscarded(t *testing.T) {
	c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) {
		return Output{
			Summary:    strings.Repeat("요", SummaryLimit/3+100),
			Suggestion: strings.Repeat("x", SuggestionLimit+500),
			Checkpoint: strings.Repeat("c", CheckpointLimit+500),
		}, nil
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	_, _ = c.Save("suggestion", profile())
	c.Observe(events("a", 1, "ask", "reply"))
	j := waitJob(t, c, "a", "completed")
	if len(j.Summary) > SummaryLimit || len(j.Suggestion) > SuggestionLimit {
		t.Fatal("not clipped:", len(j.Summary), len(j.Suggestion))
	}
	if !utf8.ValidString(j.Summary) {
		t.Fatal("clip split a rune")
	}
	for _, text := range []string{j.Summary, j.Suggestion} {
		if !strings.HasSuffix(text, "[truncated]") {
			t.Fatal("truncation was not marked:", text[max(0, len(text)-40):])
		}
	}
}

// The checkpoint is the one field fed back as context instead of read, so a
// truncated one would quietly rot what every later turn is told. It stays
// strict, and a refusal keeps the checkpoint that was already there.
func TestOverlongCheckpointIsRefused(t *testing.T) {
	c, _ := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		if in.Task == "checkpoint" {
			return Output{Checkpoint: strings.Repeat("c", CheckpointLimit+1)}, nil
		}
		return Output{Summary: "ok"}, nil
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	c.Observe(events("a", 1, strings.Repeat("a", 7000), strings.Repeat("b", 7000)))
	waitJob(t, c, "a", "completed")
	c.Observe(events("a", 4, strings.Repeat("c", 7000), strings.Repeat("d", 7000)))
	j := waitJob(t, c, "a", "failed")
	if !strings.Contains(j.Error, "exceeds") {
		t.Fatal("unhelpful refusal:", j.Error)
	}
	c.mu.Lock()
	s, e := c.load("a")
	c.mu.Unlock()
	if e != nil || s.Checkpoint != "" {
		t.Fatal("kept a truncated checkpoint", e, len(s.Checkpoint))
	}
}

func TestHistoryDoesNotBackfillAndForgetClearsDerivedData(t *testing.T) {
	c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) { return Output{Summary: "ok"}, nil })
	defer c.Close()
	_, _ = c.Save("summary", profile())
	old := events("s", 1, "old", "old")
	for _, e := range old {
		e.TimeMs = 1
	}
	c.Observe(old)
	if j, _ := c.Status("s"); j != nil {
		t.Fatal("historical backfill")
	}
	c.Observe(events("s", 4, "new", "new"))
	waitJob(t, c, "s", "completed")
	if e := c.Forget("s"); e != nil {
		t.Fatal(e)
	}
	c.Observe(events("s", 7, "deleted", "deleted"))
	if j, _ := c.Status("s"); j != nil {
		t.Fatal("deleted session reappeared")
	}
}
func TestRepeatedFailureDoesNotGrowContextWithoutLimit(t *testing.T) {
	c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) { return Output{}, errors.New("unavailable") })
	defer c.Close()
	_, _ = c.Save("summary", profile())
	for n := uint64(1); n < 100; n += 3 {
		c.Observe(events("s", n, strings.Repeat("u", 7000), strings.Repeat("a", 7000)))
		waitJob(t, c, "s", "failed")
	}
	c.mu.Lock()
	s, e := c.load("s")
	c.mu.Unlock()
	b, _ := json.Marshal(s)
	if e != nil || len(b) > 64<<10 || !s.Gap {
		t.Fatal("unbounded failure history", len(b), e)
	}
}

func TestRestartDoesNotRegeneratePrunedHistory(t *testing.T) {
	root := t.TempDir()
	c, _ := New(root, func(context.Context, Input) (Output, error) { t.Error("replayed old history"); return Output{}, nil })
	defer c.Close()
	// Simulate an old enabled config, with derived session state evicted by retention.
	c.config = Config{Summary: profile(), Since: 1}
	old := events("evicted", 10, "old paid call", "old result")
	for _, e := range old {
		e.TimeMs = time.Now().Add(-time.Minute).UnixMilli()
	}
	c.Observe(old)
	if j, _ := c.Status("evicted"); j != nil {
		t.Fatal("restart regenerated evicted turn")
	}
}
