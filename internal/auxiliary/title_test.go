package auxiliary

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
)

func waitTitle(t *testing.T, c *Controller, id string) TitleState {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
		s, err := c.Title(id)
		if err != nil {
			t.Fatal(err)
		}
		if s.Status == "completed" {
			return s
		}
		if s.Status == "failed" {
			t.Fatal(s.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("title did not finish")
	return TitleState{}
}
func TestTitleLifecycle(t *testing.T) {
	var calls atomic.Int32
	c, err := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		calls.Add(1)
		if in.Task != "title" {
			t.Errorf("unexpected task %s", in.Task)
		}
		if strings.Contains(in.Text, "tool secret") || strings.Contains(in.Text, "intermediate") {
			t.Error("included tool or intermediate response")
		}
		return Output{Title: "Session naming"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Save("title", profile()); err != nil {
		t.Fatal(err)
	}
	c.Observe(events("a", 1, "Implement session titles", "done"))
	s := waitTitle(t, c, "a")
	if s.Phase != "draft" || s.Completed != 1 {
		t.Fatalf("%+v", s)
	}
	failed := events("a", 4, "failed request", "answer")
	failed[2].Text = "failed"
	c.Observe(failed)
	second := events("a", 7, "more", "final answer")
	second = append(second[:1], append([]*api.Event{{SessionId: "a", RunId: "run", Seq: 8, TimeMs: time.Now().UnixMilli() + 1, Kind: "assistant", Text: "intermediate"}, {SessionId: "a", RunId: "run", Seq: 9, TimeMs: time.Now().UnixMilli() + 1, Kind: "tool_call", Text: "tool secret"}}, second[1:]...)...)
	second[3].Seq = 10
	second[4].Seq = 11
	c.Observe(second)
	c.Observe(events("a", 12, "finish", "complete"))
	s = waitTitle(t, c, "a")
	if s.Phase != "final" || s.Completed != 3 {
		t.Fatalf("%+v", s)
	}
	c.Observe(events("a", 15, "another topic", "ok"))
	c.Close()
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	if job, _ := c.Status("a"); job != nil {
		t.Fatal("unconfigured summary job")
	}
	again, err := New(strings.TrimSuffix(c.root, "/auxiliary"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	persisted, _ := again.Title("a")
	if persisted.Text != s.Text || persisted.Phase != "final" {
		t.Fatalf("not persisted: %+v", persisted)
	}
}
func TestTitleManualAndPurgeWinAgainstLateResult(t *testing.T) {
	for _, op := range []string{"manual", "purge", "forget"} {
		t.Run(op, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			c, _ := New(t.TempDir(), func(context.Context, Input) (Output, error) {
				close(started)
				<-release
				return Output{Title: "late"}, nil
			})
			c.Save("title", profile())
			c.Observe(events("a", 1, "request", "answer"))
			<-started
			var err error
			switch op {
			case "manual":
				err = c.SetTitle("a", "My title")
			case "purge":
				err = c.Purge("a")
			case "forget":
				err = c.Forget("a")
			}
			if err != nil {
				t.Fatal(err)
			}
			close(release)
			c.Close()
			s, _ := c.Title("a")
			if s.Text == "late" {
				t.Fatal("late result won")
			}
			if op == "manual" && s.Text != "My title" {
				t.Fatal(s)
			}
			if len(c.titleActive) != 0 {
				t.Fatal("leaked active job")
			}
			if op == "purge" {
				if _, err := os.Stat(c.titlePath("a")); !os.IsNotExist(err) {
					t.Fatal("purged title reappeared", err)
				}
			}
		})
	}
}
func TestTitleSeedAndRegenerate(t *testing.T) {
	c, _ := New(t.TempDir(), func(_ context.Context, in Input) (Output, error) {
		if len(in.Text) > MaxInput {
			t.Error("unbounded")
		}
		return Output{Title: "Regenerated"}, nil
	})
	defer c.Close()
	c.SeedTitle(&api.Session{Id: "old", CreatedAt: 1})
	if _, err := os.Stat(c.titlePath("old")); !os.IsNotExist(err) {
		t.Fatal("seeded while disabled")
	}
	c.Save("title", profile())
	c.SeedTitle(&api.Session{Id: "old", CreatedAt: 1})
	c.Observe(events("old", 1, "request", "answer"))
	s, _ := c.Title("old")
	if s.Job != "" {
		t.Fatal("backfilled old session")
	}
	if err := c.GenerateTitle("old", events("old", 1, strings.Repeat("long ", 2000), "answer")); err != nil {
		t.Fatal(err)
	}
	s = waitTitle(t, c, "old")
	if !s.Manual || s.Text != "Regenerated" {
		t.Fatal(s)
	}
}
