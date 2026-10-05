package conversation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
)

type fixture struct {
	store  *Store
	target *Session
	log    *journal.Log
	now    time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	target := &Session{ID: uuid.NewString(), Alias: "seal", RuntimeID: core.ID(), ProjectID: "project", Agent: "codex", State: "idle"}
	caller := core.ID()
	dir := core.Dir(root, target.RuntimeID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(dir, "session.json"), core.Session{ID: target.RuntimeID, ProjectID: "project"}); err != nil {
		t.Fatal(err)
	}
	log, err := journal.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	s := &Store{Root: root, Caller: caller, Project: "project", Now: func() time.Time { return now }, Registry: func(context.Context) ([]Session, error) {
		return []Session{*target, {ID: uuid.NewString(), Alias: "outside", ProjectID: "other", RuntimeID: core.ID()}}, nil
	}}
	return &fixture{s, target, log, now}
}
func (f *fixture) add(t *testing.T, kind, text string, raw []byte) uint64 {
	t.Helper()
	e, err := f.log.Append(core.Event{SessionID: f.target.RuntimeID, Kind: kind, Text: text, Raw: raw, TimeMS: f.now.Add(-5 * time.Minute).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	return e.Seq
}
func TestSearchMetadataSnapshotAndRename(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	f.add(t, "input", "Login FAILED\nlogin failed", nil)
	f.add(t, "raw", "", []byte(`{"message":"login failed"}`))
	f.add(t, "assistant", "login failed because…", nil)
	q := Query{Session: "seal", Query: "login.*failed", Match: "regex", IgnoreCase: true, Limit: 1, Around: "5m ago"}
	page, err := f.store.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].Seq != 1 || !page.HasMore || page.ID != f.target.ID || page.Alias != "seal" || page.Since == "" {
		t.Fatalf("%+v", page)
	}
	b, _ := json.Marshal(page)
	if strings.Contains(string(b), "FAILED") || strings.Contains(string(b), "because") {
		t.Fatal("search leaked body")
	}
	read, err := f.store.Read(ctx, Query{Session: page.ID, Seqs: []uint64{1}, SnapshotSeq: &page.SnapshotSeq})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(read.Bodies[0])
	if len(body) != page.Hits[0].Bytes {
		t.Fatal("size is not serialized body size")
	}
	f.target.Alias = "otter"
	f.add(t, "assistant", "login failed after snapshot", nil)
	next, err := f.store.Search(ctx, Query{Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Hits) != 1 || next.Hits[0].Seq != 3 || next.HasMore || next.Alias != "otter" || next.SnapshotSeq != 3 {
		t.Fatalf("%+v", next)
	}
	if _, err = f.store.Read(ctx, Query{Session: "seal"}); err == nil {
		t.Fatal("old alias resolved")
	}
	if _, err = f.store.Search(ctx, Query{Session: "outside"}); err == nil {
		t.Fatal("cross project read")
	}
	if _, err = f.store.Lookup(ctx, "@otter"); err == nil {
		t.Fatal("accepted mention prefix")
	}
}
func TestReadContextLinesToolsAndRaw(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "question", nil)
	f.add(t, "tool_call", "rg target", nil)
	f.add(t, "assistant", "first\nsecond\nthird", nil)
	f.add(t, "raw", "", []byte(`{"message":{"text":"native"}}`))
	f.add(t, "raw", "", []byte("partial native"))
	r, err := f.store.Read(t.Context(), Query{Session: "seal", Seqs: []uint64{3}, Before: 1})
	if err != nil || len(r.Bodies) != 2 || r.Bodies[0].Seq != 1 {
		t.Fatal(r, err)
	}
	r, err = f.store.Read(t.Context(), Query{Session: "seal", Seqs: []uint64{3}, LineStart: 2, LineEnd: 2})
	if err != nil || r.Bodies[0].Text != "second" || r.Bodies[0].LineStart != 2 {
		t.Fatal(r, err)
	}
	r, err = f.store.Search(t.Context(), Query{Session: "seal", Query: "rg", IncludeTools: true})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Seq != 2 {
		t.Fatal(r, err)
	}
	r, err = f.store.Read(t.Context(), Query{Session: "seal", View: "raw"})
	if err != nil || len(r.Bodies) != 2 || string(r.Bodies[0].Raw) != `{"message":{"text":"native"}}` || string(r.Bodies[1].RawBase64) != "partial native" {
		t.Fatal(r, err)
	}
	r, err = f.store.Read(t.Context(), Query{Session: "seal", View: "raw", Seqs: []uint64{3}})
	if err != nil || len(r.UnavailableSeqs) != 1 {
		t.Fatal(r, err)
	}
}
func TestBudgetExportAndSecretReferences(t *testing.T) {
	f := setup(t)
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("never-export-this"), 0600)
	f.add(t, "input", "[Redacted] "+secret, nil)
	f.add(t, "assistant", strings.Repeat("large text\n", 5000), nil)
	r, err := f.store.Read(t.Context(), Query{Session: "seal", Seqs: []uint64{2}})
	if err != nil || !r.HasMore || len(r.OversizedSeqs) != 1 || len(r.Bodies) != 0 {
		t.Fatal(r, err)
	}
	r, err = f.store.Read(t.Context(), Query{Cursor: r.NextCursor, Output: "file"})
	if err != nil || r.Path == "" || len(r.Bodies) != 0 || r.EventCount != 1 {
		t.Fatal(r, err)
	}
	b, err := os.ReadFile(r.Path)
	if err != nil || len(b) != r.FileBytes || strings.Count(string(b), "\n") != 1 {
		t.Fatal(len(b), err)
	}
	st, _ := os.Stat(r.Path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("export permissions", st.Mode())
	}
	first := r.Path
	for i := 0; i < 32; i++ {
		_, err = f.store.Read(t.Context(), Query{Session: "seal", Output: "file"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("old export retained", err)
	}
	r, err = f.store.Read(t.Context(), Query{Session: "seal", Output: "file"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(r.Path)
	if strings.Contains(string(b), "never-export-this") || !strings.Contains(string(b), "[Redacted]") {
		t.Fatal("secret reference dereferenced")
	}
	var meta struct {
		Reply Reply `json:"result"`
		Query Query `json:"query"`
	}
	b, _ = os.ReadFile(r.MetadataPath)
	if json.Unmarshal(b, &meta) != nil || meta.Reply.ID != f.target.ID || meta.Query.Session != f.target.ID {
		t.Fatal("missing export identity")
	}
}
func TestRetentionAndLiveTail(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "one", nil)
	f.add(t, "assistant", "two", nil)
	f.add(t, "input", "three", nil)
	page, err := f.store.Search(t.Context(), Query{Session: "seal", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: core.Snapshot{LastSeq: 2, State: "idle"}})
	if err = f.log.Compact(core.Event{SessionID: f.target.RuntimeID, Seq: 2, TimeMS: f.now.Add(-6 * time.Minute).UnixMilli(), Kind: core.HistoryCheckpointKind, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.Read(t.Context(), Query{Session: f.target.ID, Seqs: []uint64{1, 3, 99}, SnapshotSeq: &page.SnapshotSeq})
	if err != nil || !r.HistoryTruncated || r.TrimmedThrough != 2 || r.FirstSeq != 3 || len(r.MissingSeqs) != 2 || len(r.Bodies) != 1 {
		t.Fatal(r, err)
	}
	r, err = f.store.Search(t.Context(), Query{Cursor: page.NextCursor})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Seq != 3 || !r.HistoryTruncated {
		t.Fatal(r, err)
	}
	file, _ := os.OpenFile(filepath.Join(core.Dir(f.store.Root, f.target.RuntimeID), "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	file.WriteString(`[{"seq":4`)
	file.Close()
	r, err = f.store.Read(t.Context(), Query{Session: f.target.ID})
	if err != nil || len(r.Bodies) != 1 {
		t.Fatal("uncommitted tail read", r, err)
	}
}
func TestValidationAndCancellation(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "x", nil)
	for _, q := range []Query{{Session: "seal", Query: "[", Match: "regex"}, {Session: "seal", View: "other"}, {Session: "seal", Since: "yesterday"}, {Session: "seal", Around: "-5m ago"}, {Session: "seal", Around: "5m ago", Since: "2026-10-05T00:00:00Z"}, {Session: "seal", Limit: 501}, {Session: "seal", MaxBytes: -1}, {Session: "seal", LineStart: 4}, {Session: "seal", Before: 1}, {Session: "seal", Cursor: "bad"}, {Session: "../escape"}} {
		if _, err := f.store.Read(t.Context(), q); err == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.store.Read(ctx, Query{Session: "seal"}); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestTimeBounds(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "one", nil)
	stamp := f.now.Add(-5 * time.Minute).Format(time.RFC3339)
	r, err := f.store.Search(t.Context(), Query{Session: "seal", Since: stamp})
	if err != nil || len(r.Hits) != 1 {
		t.Fatal(r, err)
	}
	r, err = f.store.Search(t.Context(), Query{Session: "seal", Until: stamp})
	if err != nil || len(r.Hits) != 0 {
		t.Fatal(r, err)
	}
}

func TestExportAndJournalSymlinksStayScoped(t *testing.T) {
	f := setup(t)
	f.add(t, "input", "hello", nil)
	outside := t.TempDir()
	os.MkdirAll(core.Dir(f.store.Root, f.store.Caller), 0700)
	if err := os.Symlink(outside, filepath.Join(core.Dir(f.store.Root, f.store.Caller), "conversation-exports")); err != nil {
		t.Skip(err)
	}
	if _, err := f.store.Read(t.Context(), Query{Session: "seal", Output: "file"}); err == nil {
		t.Fatal("export followed symlink")
	}
	path := filepath.Join(core.Dir(f.store.Root, f.target.RuntimeID), "events.jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(outside, "private.jsonl")
	os.WriteFile(hidden, []byte("private"), 0600)
	if err := os.Symlink(hidden, path); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Read(t.Context(), Query{Session: "seal"}); err == nil {
		t.Fatal("journal escaped root")
	}
}
