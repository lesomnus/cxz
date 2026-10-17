package transcripthistory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"google.golang.org/protobuf/proto"
)

func fixture(t *testing.T) (*sql.DB, func(...*api.Event)) {
	t.Helper()
	db, err := func() (*sql.DB, error) {
		db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(context.Background())
		return db, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))`); err != nil {
		t.Fatal(err)
	}
	return db, func(events ...*api.Event) {
		for _, e := range events {
			e.SessionId = "s"
			b, _ := json.Marshal(e)
			if _, err := db.Exec("INSERT OR REPLACE INTO events VALUES(?,?,?)", "s", e.Seq, b); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func decode(b []byte) (*api.Event, error) {
	var e api.Event
	err := json.Unmarshal(b, &e)
	return &e, err
}
func event(seq uint64, kind, request string, payload string) *api.Event {
	return &api.Event{Seq: seq, Kind: kind, RunId: "run", RequestId: request, Payload: []byte(payload), TimeMs: int64(seq * 1000)}
}
func syncPage(t *testing.T, db *sql.DB, r *api.TranscriptRequest) *api.TranscriptReply {
	t.Helper()
	ctx := context.Background()
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	r.SessionId = "s"
	p, err := Page(ctx, db, r, decode)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestDiagnosticCardsRetainMessageButLoadOriginalDetailsLazily(t *testing.T) {
	db, add := fixture(t)
	raw := event(1, "diagnostic", "", `{"error":{"message":"Selected model is at capacity. Please try a different model.","codexErrorInfo":"serverOverloaded","additionalDetails":"`+strings.Repeat("large private detail", 1000)+`"},"willRetry":false,"threadId":"original-thread"}`)
	raw.Text = "Codex error"
	add(raw)
	page := syncPage(t, db, &api.TranscriptRequest{})
	check := func(got *api.Event) {
		t.Helper()
		data := object(got.Payload)
		if text(data, "message") != "Selected model is at capacity. Please try a different model." || text(child(data, "error"), "codexErrorInfo") != "serverOverloaded" || data["willRetry"] != false || len(got.Payload) > 300 {
			t.Fatal("missing compact error display metadata", got)
		}
	}
	check(page.Events[0])
	// An old cached projection must rebuild even when the journal is unchanged.
	if _, err := db.Exec(`UPDATE transcript_format SET version=3 WHERE session_id='s'`); err != nil {
		t.Fatal(err)
	}
	check(syncPage(t, db, &api.TranscriptRequest{}).Events[0])
	memory, err := FromEvents([]*api.Event{raw}, &api.TranscriptRequest{})
	if err != nil {
		t.Fatal(err)
	}
	check(memory.Events[0])
	details, err := Details(context.Background(), db, &api.EventDetailsRequest{SessionId: "s", Seq: 1}, decode)
	if err != nil || len(details.Events) != 1 || !proto.Equal(details.Events[0], raw) {
		t.Fatal("native error detail was changed", details, err)
	}
}
func TestSnapshotPagesAndLazyTaskDetails(t *testing.T) {
	db, add := fixture(t)
	records := []*api.Event{event(1, "input", "", ""), event(2, "tool_call", "task", `{"item":{"type":"commandExecution","command":"/bin/zsh -lc 'git status'","status":"inProgress"}}`), event(3, "approval", "approve", `{"params":{"itemId":"task"}}`), event(4, "approval_resolved", "approve", ""), event(5, "tool_output", "task", `{"content":"full output"}`), event(7, "assistant", "", "")}
	records[3].Text = "allowed"
	records[4].Text = "output"
	add(records...)
	for i := uint64(8); i < 1000; i++ {
		add(event(i, "raw", "", "{}"))
	}
	p := syncPage(t, db, &api.TranscriptRequest{Limit: 2})
	if p.SnapshotSeq != 999 || len(p.Events) != 2 || p.Events[0].Seq != 2 || p.Events[0].ToolSummary.Command != "git status" || p.Events[0].ToolSummary.State != "working" || len(p.Events[0].Payload) != 0 || !p.HasOlder || p.HasNewer || p.PrecedingInput.Seq != 1 {
		t.Fatal(p)
	}
	result := event(1000, "tool_result", "task", `{"item":{"exitCode":0,"aggregatedOutput":"full result"}}`)
	add(result)
	old := syncPage(t, db, &api.TranscriptRequest{SnapshotSeq: 999})
	current := syncPage(t, db, &api.TranscriptRequest{})
	if old.Events[1].ToolSummary.State != "working" || current.Events[1].ToolSummary.State != "completed" || current.Events[1].Seq != 2 {
		t.Fatal(old, current)
	}
	older := syncPage(t, db, &api.TranscriptRequest{BeforeSeq: 2, SnapshotSeq: 999, Limit: 2})
	if len(older.Events) != 1 || older.Events[0].Seq != 1 || older.HasOlder || !older.HasNewer {
		t.Fatal(older)
	}
	// Native details include the original command, output, and approval lifecycle;
	// reused IDs from another run must never be joined.
	other := event(1001, "tool_result", "task", "{}")
	other.RunId = "other"
	add(other)
	syncPage(t, db, &api.TranscriptRequest{})
	details, err := Details(context.Background(), db, &api.EventDetailsRequest{SessionId: "s", Seq: 2}, decode)
	if err != nil || len(details.Events) != 5 || details.Events[0].Seq != 2 || details.Events[4].Seq != 1000 || len(details.Events[0].Payload) == 0 {
		t.Fatal(details, err)
	}
	memory, err := FromEvents(append(records, result, other), &api.TranscriptRequest{SnapshotSeq: 999, Limit: 2})
	if err != nil || !proto.Equal(old.Events[1], memory.Events[0]) {
		t.Fatal(memory, err)
	}
}

func TestQuestionAnswersSurviveIncrementalIndexAndSnapshots(t *testing.T) {
	db, add := fixture(t)
	request := event(1, "approval", "q", `{"params":{"questions":[{"id":"choice","question":"Pick?","options":[{"label":"A"}]}]}}`)
	request.Text = "item/tool/requestUserInput"
	add(request)
	initial := syncPage(t, db, &api.TranscriptRequest{})
	if len(initial.Events) != 1 || object(initial.Events[0].Payload)["cxz_resolution"] != nil {
		t.Fatal(initial)
	}
	resolved := event(2, "approval_resolved", "q", `{"state":"allowed","answers":{"choice":{"selected":["A"],"other":""}}}`)
	resolved.Text = "allowed"
	add(resolved)
	current := syncPage(t, db, &api.TranscriptRequest{})
	if len(current.Events) != 1 || current.Events[0].Seq != 1 || !strings.Contains(string(current.Events[0].Payload), `"selected":["A"]`) {
		t.Fatal(current)
	}
	old := syncPage(t, db, &api.TranscriptRequest{SnapshotSeq: 1})
	if object(old.Events[0].Payload)["cxz_resolution"] != nil || strings.Contains(string(request.Payload), "cxz_resolution") {
		t.Fatal("snapshot or native journal mutated")
	}
	memory, err := FromEvents([]*api.Event{request, resolved}, &api.TranscriptRequest{})
	if err != nil || !proto.Equal(memory.Events[0], current.Events[0]) {
		t.Fatal(memory, err)
	}
	// Reused request IDs in another run must not resolve the original question.
	other := event(3, "approval_resolved", "q", "{}")
	other.RunId, other.Text = "other", "denied"
	add(other)
	if got := syncPage(t, db, &api.TranscriptRequest{}); !proto.Equal(got.Events[0], current.Events[0]) {
		t.Fatal(got)
	}
}
func TestBackfillRetentionAndCancellation(t *testing.T) {
	db, add := fixture(t)
	add(event(10, "input", "", ""), event(30, "assistant", "", ""))
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 2 {
		t.Fatal(p)
	}
	add(event(20, "assistant", "", "")) // fills a hole without changing the high-water mark
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 3 || p.Events[1].Seq != 20 {
		t.Fatal(p)
	}
	db.Exec("DELETE FROM events WHERE seq<30")
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 1 || p.HasOlder || p.PrecedingInput != nil {
		t.Fatal(p)
	}
	add(event(40, "assistant", "", ""))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sync(ctx, db, "s", decode); err == nil {
		t.Fatal("canceled index must not commit")
	}
	if p := syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 2 {
		t.Fatal(p)
	}
}
func TestCompletionAndMetadata(t *testing.T) {
	db, add := fixture(t)
	input := event(1, "input", "", "")
	assistant := event(2, "assistant", "", `{"item":{"phase":"final_answer"}}`)
	assistant.Response = &api.ResponseMetadata{Model: "model", Effort: "high", Phase: "final_answer"}
	end := event(3, "turn_end", "", `{"duration_ms":1200,"usage":{"input_tokens":100,"cache_read_input_tokens":20},"total_cost_usd":0.01}`)
	end.Text = "completed"
	add(input, assistant, end, event(4, "models", "", "{}"), event(5, "usage", "", "{}"))
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 2 || len(p.Metadata) != 2 || p.Events[1].Response.Model != "model" {
		t.Fatal(p)
	}
	c := object(p.Events[1].Response.CompletionJson)
	if c["duration_source"] != "provider" || c["duration_ms"] != float64(1200) || child(c, "metrics")["cache_read_tokens"] != float64(20) {
		t.Fatal(c)
	}
	// Incremental restore must not attach an empty turn to a previously completed response.
	next := event(6, "turn_end", "", "")
	next.Text = "completed"
	add(next)
	if p = syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 3 || p.Events[2].Kind != "turn_end" {
		t.Fatal(p)
	}
	commentary := event(7, "assistant", "", `{"item":{"phase":"commentary"}}`)
	add(commentary)
	next = event(8, "turn_end", "", "")
	next.Text = "completed"
	add(next)
	if p = syncPage(t, db, &api.TranscriptRequest{}); len(p.Events) != 5 || p.Events[4].Kind != "turn_end" {
		t.Fatal(p)
	}
}

func TestFileChangesEnrichOriginalRowAndUpgradeCachedProjection(t *testing.T) {
	db, add := fixture(t)
	start := event(1, "tool_call", "files", `{"item":{"type":"fileChange","status":"inProgress","changes":[]}}`)
	add(start)
	before := syncPage(t, db, &api.TranscriptRequest{})
	if len(before.Events) != 1 || len(before.Events[0].ToolSummary.Files) != 0 {
		t.Fatal(before)
	}
	end := event(2, "tool_result", "files", `{"item":{"type":"fileChange","status":"completed","changes":[{"path":"/workspace/ts/src/app.tsx","kind":{"type":"update","move_path":"app-new.tsx"},"diff":"--- a/app.tsx\n+++ b/app-new.tsx\n@@ -1 +1,2 @@\n-old\n+new\n+SECRET_DIFF_BODY\n"},{"path":"/workspace/ts/src/send-motion.ts","kind":{"type":"add"}},{"path":"/workspace/ts/src/style.css","kind":"delete"}]}}`)
	add(end)
	page := syncPage(t, db, &api.TranscriptRequest{})
	row := page.Events[0]
	if len(page.Events) != 1 || row.Seq != 1 || row.ToolSummary.State != "completed" || len(row.ToolSummary.Files) != 3 {
		t.Fatal(page)
	}
	f := row.ToolSummary.Files[0]
	if f.Path != "/workspace/ts/src/app.tsx" || f.MovePath != "app-new.tsx" || f.Action != "update" || f.Added != 2 || f.Removed != 1 || f.Measure != "diff" {
		t.Fatal(f)
	}
	if row.ToolSummary.Files[1].Measure != "unknown" || row.ToolSummary.Files[1].Action != "add" || row.ToolSummary.Files[2].Action != "delete" {
		t.Fatal(row)
	}
	encoded, _ := json.Marshal(row)
	if len(row.Payload) != 0 || strings.Contains(string(encoded), "SECRET_DIFF_BODY") {
		t.Fatal("summary leaked file body")
	}
	old := syncPage(t, db, &api.TranscriptRequest{SnapshotSeq: 1})
	if len(old.Events[0].ToolSummary.Files) != 0 || old.Events[0].ToolSummary.State != "working" {
		t.Fatal("snapshot fence changed", old)
	}
	// Model an index made by an older build with identical native bounds.
	row.ToolSummary.Files = nil
	legacy, _ := json.Marshal(row)
	if _, err := db.Exec(`UPDATE transcript_rows SET data=? WHERE session_id='s' AND updated=2`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM transcript_format WHERE session_id='s'`); err != nil {
		t.Fatal(err)
	}
	rebuilt := syncPage(t, db, &api.TranscriptRequest{})
	if len(rebuilt.Events[0].ToolSummary.Files) != 3 {
		t.Fatal("old cached projection was not rebuilt", rebuilt)
	}
	details, err := Details(context.Background(), db, &api.EventDetailsRequest{SessionId: "s", Seq: 1}, decode)
	if err != nil || len(details.Events) != 2 || !strings.Contains(string(details.Events[1].Payload), "SECRET_DIFF_BODY") {
		t.Fatal(details, err)
	}
}

func TestFileSummaryCountsAreRecordedSpansAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name, payload, measure string
		added, removed, lines  uint32
		perMatch               bool
	}{
		{"Write", `{"file_path":"a","content":"one\ntwo\n"}`, "content", 0, 0, 2, false},
		{"Edit", `{"input":{"file_path":"a","old_string":"one\ntwo","new_string":"new","replace_all":true}}`, "replacement", 1, 2, 0, true},
		{"Write", `{"file_path":"a"}`, "unknown", 0, 0, 0, false},
		{"Read", `{"file_path":"a"}`, "", 0, 0, 0, false},
	} {
		e := event(1, "tool_call", "file", tc.payload)
		e.Text = tc.name
		f := summary(e).ToolSummary.Files[0]
		if f.Path != "a" || f.Measure != tc.measure || f.Added != tc.added || f.Removed != tc.removed || f.Lines != tc.lines || f.PerMatch != tc.perMatch {
			t.Fatal(tc.name, f)
		}
	}
	p := New()
	call := event(1, "tool_call", "write", `{"file_path":"a","content":"one\ntwo\n"}`)
	call.Text = "Write"
	result := event(2, "tool_result", "write", `{"content":"Write"}`)
	result.Text = "Write"
	p.Apply(call)
	p.Apply(result)
	if f := p.Rows[1].ToolSummary.Files[0]; f.Path != "a" || f.Lines != 2 || f.Measure != "content" {
		t.Fatal("result text was mistaken for a new file request", f)
	}
	var changes []map[string]any
	for range 300 {
		changes = append(changes, map[string]any{"path": strings.Repeat("x", 1100), "kind": "add"})
	}
	payload, _ := json.Marshal(map[string]any{"item": map[string]any{"type": "fileChange", "changes": changes}})
	e := event(1, "tool_call", "many", string(payload))
	s := summary(e).ToolSummary
	if len(s.Files) != 256 || s.OmittedFiles != 44 || len([]rune(s.Files[0].Path)) != 1025 {
		t.Fatal("unbounded summary", len(s.Files), s.OmittedFiles)
	}
}

// The stream and the page are one projection, which is the property every
// client that moves onto Rows depends on. Pinned here rather than asserted at
// each call site: a row a reader was streamed and the same row it later pages
// in have to be the same bytes, or the reader has to tell them apart -- which
// is the dual-shape problem Rows exists to remove.
func TestTailAndPageAgreeOnEveryRow(t *testing.T) {
	db, add := fixture(t)
	input := event(1, "input", "", "")
	call := event(2, "tool_call", "r1", `{"name":"read"}`)
	result := event(3, "tool_result", "r1", `{"ok":true}`)
	assistant := event(4, "assistant", "", `{"item":{"phase":"final_answer"}}`)
	assistant.Response = &api.ResponseMetadata{Model: "model", Phase: "final_answer"}
	end := event(5, "turn_end", "", `{"duration_ms":90,"usage":{"input_tokens":7},"total_cost_usd":0.5}`)
	end.Text = "completed"
	add(input, call, result, assistant, end)

	page := syncPage(t, db, &api.TranscriptRequest{})
	current := map[uint64]*api.Event{}
	var order []uint64
	for _, v := range tail(t, db, 0) {
		if _, seen := current[v.Event.Seq]; !seen {
			order = append(order, v.Event.Seq)
		}
		current[v.Event.Seq] = v.Event
	}
	if len(order) != len(page.Events) {
		t.Fatalf("stream had %d rows, page had %d", len(order), len(page.Events))
	}
	for i, seq := range order {
		want := page.Events[i]
		if got := current[seq]; !proto.Equal(got, want) {
			t.Fatalf("row %d differs\nstream: %v\npage:   %v", seq, got, want)
		}
	}

	// The turn's completion arrives as a second version of the response row,
	// after the version without it. A reader replaces by seq; it is never asked
	// to fold anything itself.
	versions := tail(t, db, 0)
	var response []Version
	for _, v := range versions {
		if v.Event.Seq == assistant.Seq {
			response = append(response, v)
		}
	}
	if len(response) != 2 {
		t.Fatalf("the response row was published %d times, want 2", len(response))
	}
	if response[0].Updated >= response[1].Updated || response[1].Updated != end.Seq {
		t.Fatal("the correction did not arrive after the row, keyed by the completing record", response[0].Updated, response[1].Updated)
	}
	if len(object(response[0].Event.GetResponse().GetCompletionJson())) != 0 {
		t.Fatal("the first version already carried a completion")
	}
	if object(response[1].Event.GetResponse().GetCompletionJson())["duration_ms"] != float64(90) {
		t.Fatal("the second version did not carry the turn's metrics")
	}

	// No turn_end row of its own: the metrics are on the response. A client that
	// kept the record would be holding the shape this removes.
	for _, v := range versions {
		if v.Event.Kind == "turn_end" {
			t.Fatal("a completed turn kept its record as a row")
		}
	}

	// Resuming from a cursor yields what came after it and nothing else: from
	// the response row's first version, that is the correction alone.
	from := tail(t, db, response[0].Updated)
	if len(from) != 1 || from[0].Updated != end.Seq || from[0].Event.Seq != assistant.Seq {
		t.Fatalf("resume from a cursor replayed or skipped: %v", from)
	}
	if last := tail(t, db, end.Seq); len(last) != 0 {
		t.Fatalf("resume from the end replayed %d rows", len(last))
	}
}

func tail(t *testing.T, db *sql.DB, after uint64) []Version {
	t.Helper()
	if err := Sync(context.Background(), db, "s", decode); err != nil {
		t.Fatal(err)
	}
	rows, err := Tail(context.Background(), db, "s", after, 0, decode)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]Version, 0, len(rows))
	for _, r := range rows {
		out = append(out, Version{Seq: r.Event.Seq, Updated: r.Updated, Event: r.Event})
	}
	return out
}

// These five behaviours had their only coverage in TypeScript, where this
// reduction used to be duplicated. The implementation is here now, so the
// assertions are too -- moving the code without moving its tests is how the
// behaviour would quietly change. See cxz#155.

// A tool's row moves through the states its records describe, and the records
// stay available behind it rather than being consumed. These are the
// projection's own states; what a card displays for a given agent is a
// separate mapping, tested where that mapping lives.
func TestToolRowAdvancesThroughApprovalToCompletion(t *testing.T) {
	db, add := fixture(t)
	call := event(1, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"git status"}}`)
	add(call)
	state := func() string {
		p := syncPage(t, db, &api.TranscriptRequest{})
		if len(p.Events) != 1 || p.Events[0].Seq != 1 {
			t.Fatalf("a tool's records produced %d rows, want its call", len(p.Events))
		}
		return p.Events[0].GetToolSummary().GetState()
	}
	// A call that has reported nothing yet is pending rather than working: no
	// output has arrived and no approval has been asked for.
	if got := state(); got != "pending" {
		t.Fatal("a call alone did not read as pending:", got)
	}
	approval := event(2, "approval", "approve-1", `{"tool_use_id":"exec-1","item":{"type":"commandExecution"}}`)
	add(approval)
	if got := state(); got != "pending" {
		t.Fatal("an approval request moved a pending row:", got)
	}
	allowed := event(3, "approval_resolved", "approve-1", `{}`)
	allowed.Text = "allowed"
	add(allowed)
	if got := state(); got != "working" {
		t.Fatal("an allowed approval did not release the row:", got)
	}
	result := event(4, "tool_result", "exec-1", `{"item":{"type":"commandExecution","status":"completed","exitCode":0}}`)
	add(result)
	if got := state(); got != "completed" {
		t.Fatal("the result did not complete the row:", got)
	}
	// The records are still there for a card that wants the output.
	batch, err := Details(context.Background(), db, &api.EventDetailsRequest{SessionId: "s", Seq: 1}, decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) != 4 {
		t.Fatalf("details returned %d records, want the call, approval, resolution and result", len(batch.Events))
	}
}

// A refused approval fails the row rather than leaving it working.
func TestDeclinedApprovalFailsTheToolRow(t *testing.T) {
	db, add := fixture(t)
	add(event(1, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"rm -rf /"}}`))
	add(event(2, "approval", "approve-1", `{"tool_use_id":"exec-1"}`))
	denied := event(3, "approval_resolved", "approve-1", `{}`)
	denied.Text = "denied"
	add(denied)
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 1 || p.Events[0].GetToolSummary().GetState() != "failed" {
		t.Fatal("a denied approval did not fail the row", p.Events)
	}
}

// A nonzero exit is a failure, which the row says rather than the reader
// inferring it from a payload.
func TestNonzeroExitFailsTheToolRow(t *testing.T) {
	db, add := fixture(t)
	add(event(1, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"false"}}`))
	add(event(2, "tool_result", "exec-1", `{"item":{"type":"commandExecution","status":"failed","exitCode":1}}`))
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 1 || p.Events[0].GetToolSummary().GetState() != "failed" {
		t.Fatal("a nonzero exit did not fail the row", p.Events)
	}
}

// Request identity is per run. The same command in another run is another
// tool, and a result from another run does not complete this one.
func TestToolIdentityDoesNotCrossRuns(t *testing.T) {
	db, add := fixture(t)
	add(event(1, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"git status"}}`))
	same := event(2, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"git status"}}`)
	same.RunId = "other-run"
	add(same)
	elsewhere := event(3, "tool_result", "exec-1", `{"item":{"status":"completed"}}`)
	elsewhere.RunId = "other-run"
	add(elsewhere)
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 2 {
		t.Fatalf("two runs produced %d rows, want one each", len(p.Events))
	}
	if p.Events[0].GetToolSummary().GetState() == "completed" {
		t.Fatal("another run's result completed this run's tool")
	}
	if p.Events[1].GetToolSummary().GetState() != "completed" {
		t.Fatal("a run's own result did not complete its tool")
	}
}

// Output records interleave with a response and are not rows; the row is
// republished once to say the tool is working, not once per chunk.
func TestInterleavedOutputDoesNotRepublishPerChunk(t *testing.T) {
	db, add := fixture(t)
	add(event(1, "tool_call", "exec-1", `{"item":{"type":"commandExecution","command":"git status"}}`))
	add(event(2, "tool_output", "exec-1", `{}`))
	add(event(3, "assistant", "", `{"item":{"phase":"commentary"}}`))
	add(event(4, "tool_output", "exec-1", `{}`))
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 2 || p.Events[0].Seq != 1 || p.Events[1].Seq != 3 {
		t.Fatalf("output records became rows: %v", p.Events)
	}
	versions, err := Tail(context.Background(), db, "s", 0, 0, decode)
	if err != nil {
		t.Fatal(err)
	}
	published := 0
	for _, v := range versions {
		if v.Event.Seq == 1 {
			published++
		}
	}
	if published != 2 {
		t.Fatalf("the tool row was published %d times, want its call and one working update", published)
	}
}

// A result whose call has been trimmed away still reads, rather than being
// dropped for having no anchor.
func TestOrphanToolRecordsStillRead(t *testing.T) {
	db, add := fixture(t)
	add(event(4, "tool_result", "exec-1", `{"item":{"type":"commandExecution","command":"git status","status":"completed"}}`))
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != 1 || p.Events[0].Seq != 4 {
		t.Fatalf("an orphan result was dropped: %v", p.Events)
	}
	if got := p.Events[0].GetToolSummary().GetState(); got != "completed" {
		t.Fatal("an orphan result lost its state:", got)
	}
}

// A row says what shape it is, because a reader lays out a command, a file list
// and a bare tool differently and cannot work it out from the rest of the row:
// a Read puts the path it read in Command as well as in Files.
func TestRowSaysWhatShapeAToolIs(t *testing.T) {
	db, add := fixture(t)
	calls := []struct {
		name, payload, kind, description string
	}{
		{"Read", `{"input":{"file_path":"ts/src/app/app.tsx"}}`, "read", ""},
		{"Bash", `{"input":{"command":"/usr/bin/zsh -lc 'ls -la'","description":"List the tree"}}`, "command", "List the tree"},
		{"Skill", `{"input":{"skill":"dataviz","args":"palette"}}`, "tool", "Skill dataviz palette"},
		{"cxz_memory_search", `{"input":{"query":"anything"}}`, "tool", ""},
	}
	for i, c := range calls {
		e := event(uint64(i+1), "tool_call", fmt.Sprintf("call-%d", i), c.payload)
		e.Text = c.name
		add(e)
	}
	p := syncPage(t, db, &api.TranscriptRequest{})
	if len(p.Events) != len(calls) {
		t.Fatalf("%d calls produced %d rows", len(calls), len(p.Events))
	}
	for i, c := range calls {
		label := p.Events[i].GetToolSummary()
		if label.GetKind() != c.kind {
			t.Fatalf("%s is shaped %q, want %q", c.name, label.GetKind(), c.kind)
		}
		if label.GetDescription() != c.description {
			t.Fatalf("%s describes itself as %q, want %q", c.name, label.GetDescription(), c.description)
		}
	}
	// The Read is the reason kind is carried rather than inferred: Command is
	// set, so a reader guessing from it would draw a shell.
	if read := p.Events[0].GetToolSummary(); read.GetCommand() == "" {
		t.Fatal("a Read lost the path it read")
	}
	// A call the projection cannot parse says it is a tool and says nothing
	// else: its name is all a reader gets, and a description repeating the name
	// would be a reader thinking it had more than it does.
	if last := p.Events[3].GetToolSummary(); last.GetName() != "cxz_memory_search" {
		t.Fatal("an unparsed call lost its name:", last.GetName())
	}
}

// A page carries records beside its rows for the things a client derives rather
// than reads: quota, the model catalog, the compact boundary. Each of those is a
// scan backwards for the latest record of one kind, so the budget is per kind --
// a session that writes a usage record every turn must not evict the single
// `models` record it wrote at the start.
func TestMetadataKeepsTheRareKindsAChattySessionWouldEvict(t *testing.T) {
	db, add := fixture(t)
	add(event(1, "models", "", `{"catalog":"first"}`))
	add(event(2, "compact", "", `{}`))
	status := event(3, "usage_status", "", `{}`)
	status.Text = "polling"
	add(status)
	for i := 0; i < metadataPerKind*4; i++ {
		add(event(uint64(10+i), "usage", "", `{"n":1}`))
	}
	p := syncPage(t, db, &api.TranscriptRequest{})
	counts := map[string]int{}
	for _, e := range p.Metadata {
		counts[e.Kind]++
	}
	if counts["models"] != 1 || counts["compact"] != 1 {
		t.Fatalf("a chatty kind evicted a rare one: %v", counts)
	}
	// Quota's state is only in these records: a client with usage but no status
	// cannot tell "no limits reported" from "limits not asked for yet".
	if counts["usage_status"] != 1 {
		t.Fatalf("usage_status did not reach the page: %v", counts)
	}
	if counts["usage"] != metadataPerKind {
		t.Fatalf("usage was kept %d deep, want %d", counts["usage"], metadataPerKind)
	}
	// Ascending by seq, which is how a reader merges them, and the newest of the
	// chatty kind is the one that survived.
	for i := 1; i < len(p.Metadata); i++ {
		if p.Metadata[i-1].Seq >= p.Metadata[i].Seq {
			t.Fatal("metadata is not in reading order", p.Metadata[i-1].Seq, p.Metadata[i].Seq)
		}
	}
	if last := p.Metadata[len(p.Metadata)-1]; last.Seq != uint64(9+metadataPerKind*4) {
		t.Fatal("the oldest records were kept instead of the newest:", last.Seq)
	}
	// The in-memory path answers the same page, because the sandbox and the
	// browser fixture read it and a reader cannot tell which one served it.
	var all []*api.Event
	rows, err := db.Query(`SELECT data FROM events WHERE session_id='s' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		e, x := decode(b)
		if x != nil {
			t.Fatal(x)
		}
		all = append(all, e)
	}
	rows.Close()
	memory, err := FromEvents(all, &api.TranscriptRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.Metadata) != len(p.Metadata) {
		t.Fatalf("the in-memory page carried %d metadata records, the indexed one %d", len(memory.Metadata), len(p.Metadata))
	}
	for i := range memory.Metadata {
		if memory.Metadata[i].Seq != p.Metadata[i].Seq {
			t.Fatalf("the two paths kept different records at %d: %d and %d", i, memory.Metadata[i].Seq, p.Metadata[i].Seq)
		}
	}
}
