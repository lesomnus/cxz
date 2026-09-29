package auxiliary

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
)

func TestSessionOverridesPersistWithoutChangingDefaults(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int32
	runner := func(_ context.Context, in Input) (Output, error) { calls.Add(1); return Output{Summary: "요약"}, nil }
	c, err := New(root, runner)
	if err != nil {
		t.Fatal(err)
	}
	p := profile()
	p.Enabled = false
	if _, err = c.Save("summary", p); err != nil {
		t.Fatal(err)
	}
	if err = c.SetSession("a", "summary", true); err != nil {
		t.Fatal(err)
	}
	c.Observe(events("a", 1, "user", "answer"))
	waitJob(t, c, "a", "completed")
	c.Observe(events("b", 1, "user", "answer"))
	if j, _ := c.Status("b"); j != nil {
		t.Fatal("session preference leaked")
	}
	if err = c.Generate("a", "summary", nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("bare enabled command generated")
	}
	if c.Config().Summary.Enabled {
		t.Fatal("changed global default")
	}
	c.Close()
	c, err = New(root, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	cfg, err := c.SessionConfig("a")
	if err != nil || !cfg.Summary {
		t.Fatal("override lost", cfg, err)
	}
	if err = c.SetSession("a", "summary", false); err != nil {
		t.Fatal(err)
	}
	c.Observe(events("a", 4, "next", "answer"))
	if calls.Load() != 1 {
		t.Fatal("disabled generated")
	}
	summaries, _ := c.Summaries("a")
	if len(summaries) != 1 || summaries[0].Text != "요약" {
		t.Fatal("prior summary lost", summaries)
	}
}

func TestOneShotUsesFinalResponseAndSameTurnSummary(t *testing.T) {
	var inputs []Input
	c, err := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		inputs = append(inputs, in)
		if in.Task == "summary" {
			return Output{Summary: "같은 턴 요약"}, nil
		}
		return Output{Suggestion: "계속 진행해줘"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := profile()
	p.Enabled = false
	_, _ = c.Save("summary", p)
	_, _ = c.Save("suggestion", p)
	history := events("a", 1, "사용자 질문", "final")
	history[1].Text = "commentary"
	history = append(history[:2], &api.Event{SessionId: "a", RunId: "run", Seq: 3, Kind: "tool_call"}, &api.Event{SessionId: "a", RunId: "run", Seq: 4, Kind: "assistant", Text: "final"}, &api.Event{SessionId: "a", RunId: "run", Seq: 5, Kind: "turn_end", Text: "completed"})
	for _, e := range history {
		e.TimeMs = 1
	} // explicit old-history opt-in, never automatic replay
	if err = c.Generate("a", "summary", history); err != nil {
		t.Fatal(err)
	}
	waitJob(t, c, "a", "completed")
	if err = c.Generate("a", "suggestion", history); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, c, "a", "completed")
	if len(inputs) != 2 || !strings.Contains(inputs[1].Text, "같은 턴 요약") || strings.Contains(inputs[0].Text, "commentary") {
		t.Fatal(inputs)
	}
	if j.Summary != "같은 턴 요약" || j.Suggestion == "" {
		t.Fatal(j)
	}
	cfg, _ := c.SessionConfig("a")
	if cfg.Summary || cfg.Suggestion {
		t.Fatal("one shot enabled automatic tasks")
	}
}

func TestSeparateProfilesPublishSummaryBeforeSuggestion(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	c, _ := New(t.TempDir(), func(ctx context.Context, in Input) (Output, error) {
		if in.Task == "summary" {
			return Output{Summary: "visible immediately"}, nil
		}
		if !strings.Contains(in.Text, "visible immediately") {
			t.Error("missing summary context")
		}
		close(waiting)
		select {
		case <-release:
			return Output{Suggestion: "next"}, nil
		case <-ctx.Done():
			return Output{}, ctx.Err()
		}
	})
	defer c.Close()
	p := profile()
	_, _ = c.Save("summary", p)
	p.Model = "other"
	_, _ = c.Save("suggestion", p)
	c.Observe(events("a", 1, "user", "answer"))
	select {
	case <-waiting:
	case <-time.After(3 * time.Second):
		t.Fatal("suggestion not started")
	}
	j, err := c.Status("a")
	if err != nil || j.Status != "running" || j.Summary != "visible immediately" {
		t.Fatal(j, err)
	}
	close(release)
	waitJob(t, c, "a", "completed")
}

func TestOneShotRejectsIncompleteOrSupersededHistory(t *testing.T) {
	var calls atomic.Int32
	c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) { calls.Add(1); return Output{}, nil })
	defer c.Close()
	p := profile()
	p.Enabled = false
	_, _ = c.Save("summary", p)
	history := events("a", 1, "user", "answer")
	for _, batch := range [][]*api.Event{history[1:], append(history, &api.Event{SessionId: "a", RunId: "run", Seq: 4, Kind: "input", Text: "new"}), events("other", 1, "user", "answer")} {
		if err := c.Generate("a", "summary", batch); err == nil {
			t.Fatal("accepted incomplete/foreign source")
		}
	}
	c.Observe(events("a", 4, "new", "answer")[:1])
	if err := c.Generate("a", "summary", history); err == nil {
		t.Fatal("accepted superseded source")
	}
	if calls.Load() != 0 {
		t.Fatal("unexpected calls")
	}
}

func TestDisableCancelsAndSummaryStorageIsBounded(t *testing.T) {
	started := make(chan struct{})
	c, _ := New(t.TempDir(), func(ctx context.Context, in Input) (Output, error) {
		close(started)
		<-ctx.Done()
		return Output{}, ctx.Err()
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	c.Observe(events("a", 1, "user", "answer"))
	<-started
	if err := c.SetSession("a", "summary", false); err != nil {
		t.Fatal(err)
	}
	waitJob(t, c, "a", "canceled")
	var s State
	for i := uint64(1); i <= 40; i++ {
		rememberSummary(&s, &Job{Run: "run", Turn: i, Summary: strings.Repeat("한", 2000)})
	}
	if len(s.Summaries) != 32 || s.Summaries[0].Turn != 9 || len(s.Summaries[0].Text) > 4<<10 {
		t.Fatal("unbounded summary storage")
	}
}

func TestQueuedTaskDisabledBeforeSlotNeverCallsProvider(t *testing.T) {
	var calls atomic.Int32
	c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) { calls.Add(1); return Output{Summary: "unexpected"}, nil })
	for i := 0; i < cap(c.slots); i++ {
		c.slots <- struct{}{}
	}
	_, _ = c.Save("summary", profile())
	c.Observe(events("a", 1, "question", "answer"))
	if err := c.SetSession("a", "summary", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(c.slots); i++ {
		<-c.slots
	}
	c.Close()
	if calls.Load() != 0 {
		t.Fatal("disabled queued task still called provider")
	}
}

func TestOneShotSuggestionQueuesBehindRunningSummary(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c, _ := New(t.TempDir(), func(ctx context.Context, in Input) (Output, error) {
		calls.Add(1)
		if in.Task == "summary" {
			close(started)
			select {
			case <-release:
				return Output{Summary: "finished summary"}, nil
			case <-ctx.Done():
				return Output{}, ctx.Err()
			}
		}
		if !strings.Contains(in.Text, "finished summary") {
			t.Error("queued suggestion lost summary")
		}
		return Output{Suggestion: "next"}, nil
	})
	defer c.Close()
	_, _ = c.Save("summary", profile())
	p := profile()
	p.Enabled = false
	_, _ = c.Save("suggestion", p)
	history := events("a", 1, "question", "answer")
	c.Observe(history)
	<-started
	if err := c.Generate("a", "suggestion", history); err != nil {
		t.Fatal(err)
	}
	if err := c.Generate("a", "suggestion", history); err != nil {
		t.Fatal(err)
	} // deduplicated while queued
	j, _ := c.Status("a")
	if !j.SuggestionRequested || j.Status != "running" {
		t.Fatal(j)
	}
	close(release)
	j = waitJob(t, c, "a", "completed")
	if j.Summary != "finished summary" || j.Suggestion != "next" || calls.Load() != 2 {
		t.Fatal(j, calls.Load())
	}
	cfg, _ := c.SessionConfig("a")
	if cfg.Suggestion {
		t.Fatal("one shot changed preference")
	}
}
