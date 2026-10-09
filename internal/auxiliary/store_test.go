package auxiliary

import (
	"fmt"
	"testing"
)

func testStore(t *testing.T) *store {
	t.Helper()
	s, err := openStore(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// A task used to be one pointer in a file, so the next turn overwrote it. Now
// each one stays, with the results it produced.
func TestTaskHistoryOutlivesTheTurn(t *testing.T) {
	s := testStore(t)
	for turn := uint64(1); turn <= 3; turn++ {
		j := &Job{
			ID: fmt.Sprintf("job%d", turn), Run: "run", Turn: turn, Revision: "r1",
			Status: "completed", SummaryRequested: true, SuggestionRequested: true,
			Summary: fmt.Sprintf("summary %d", turn), Suggestion: "do the next thing",
			Usage:   []Usage{{Account: "work", Model: "m", Task: "combined"}},
		}
		if err := s.putTask("a", j, int64(turn)); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := s.tasks("a", 0)
	if err != nil || len(tasks) != 3 {
		t.Fatal(tasks, err)
	}
	// Newest first, with what it was asked for and what came back.
	if tasks[0].Turn != 3 || tasks[0].Summary != "summary 3" || tasks[0].Suggestion != "do the next thing" {
		t.Fatal(tasks[0])
	}
	if !tasks[0].SummaryRequested || !tasks[0].SuggestionRequested || len(tasks[0].Usage) != 1 {
		t.Fatal("kinds or usage lost", tasks[0])
	}
	// The same task written again is the same row, not a second one.
	again := tasks[0]
	again.Status, again.Suggestion = "stale", ""
	if err = s.putTask("a", &again, 4); err != nil {
		t.Fatal(err)
	}
	tasks, _ = s.tasks("a", 0)
	if len(tasks) != 3 || tasks[0].Status != "stale" {
		t.Fatal("a rewrite added a row", len(tasks), tasks[0].Status)
	}
	// A result that no longer stands is removed rather than kept as an answer.
	if tasks[0].Suggestion != "" {
		t.Fatal("a dropped suggestion was kept", tasks[0].Suggestion)
	}
}

// One session cannot grow the store without end, and neither can a manager
// that has seen many sessions.
func TestStoreIsBounded(t *testing.T) {
	s := testStore(t)
	for i := 0; i < TaskHistory+20; i++ {
		j := &Job{ID: fmt.Sprintf("job%d", i), Run: "run", Turn: uint64(i), Status: "completed", SummaryRequested: true, Summary: "s"}
		if err := s.putTask("a", j, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := s.tasks("a", TaskHistory)
	if err != nil || len(tasks) != TaskHistory {
		t.Fatal(len(tasks), err)
	}
	var results int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM results`).Scan(&results); err != nil {
		t.Fatal(err)
	}
	if results != TaskHistory {
		t.Fatal("results outlived their tasks", results)
	}
	for i := 0; i < SessionHistory+4; i++ {
		v := Summary{Run: "run", Turn: 1, Text: "s"}
		if err = s.putSummary(fmt.Sprintf("session%03d", i), v, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	var sessions int
	if err = s.db.QueryRow(`SELECT COUNT(DISTINCT session) FROM summaries`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != SessionHistory {
		t.Fatal("unbounded number of sessions", sessions)
	}
}

// Summaries read in the order a transcript reads them, and a window skips what
// the caller already has.
func TestSummariesReadOldestFirst(t *testing.T) {
	s := testStore(t)
	for turn := uint64(1); turn <= 5; turn++ {
		if err := s.putSummary("a", Summary{Run: "run", Turn: turn, Text: fmt.Sprint(turn)}, int64(turn)); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.summaries("a", 0, 0)
	if err != nil || len(all) != 5 || all[0].Turn != 1 || all[4].Turn != 5 {
		t.Fatal(all, err)
	}
	after, _ := s.summaries("a", 3, 0)
	if len(after) != 2 || after[0].Turn != 4 {
		t.Fatal(after)
	}
	// A limit keeps the newest, and still reads oldest first.
	limited, _ := s.summaries("a", 0, 2)
	if len(limited) != 2 || limited[0].Turn != 4 || limited[1].Turn != 5 {
		t.Fatal(limited)
	}
}

// A session that is gone takes its rows with it, and leaves everyone else's.
func TestForgetRemovesOnlyThatSession(t *testing.T) {
	s := testStore(t)
	for _, id := range []string{"a", "b"} {
		if err := s.putTask(id, &Job{ID: "job-" + id, Run: "run", Turn: 1, Status: "completed", SummaryRequested: true, Summary: "s"}, 1); err != nil {
			t.Fatal(err)
		}
		if err := s.putSummary(id, Summary{Run: "run", Turn: 1, Text: "s"}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.forget("a"); err != nil {
		t.Fatal(err)
	}
	if tasks, _ := s.tasks("a", 0); len(tasks) != 0 {
		t.Fatal("tasks survived", tasks)
	}
	if sums, _ := s.summaries("a", 0, 0); len(sums) != 0 {
		t.Fatal("summaries survived", sums)
	}
	var results int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM results WHERE task = 'job-a'`).Scan(&results); err != nil || results != 0 {
		t.Fatal("results survived", results, err)
	}
	if tasks, _ := s.tasks("b", 0); len(tasks) != 1 {
		t.Fatal("another session was taken too", tasks)
	}
}
