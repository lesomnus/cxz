package sessionfork_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/sessionfork"
	"github.com/lesomnus/cxz/internal/supervisor"
)

func fixture(t *testing.T, agent string) (string, core.Session, []core.Event) {
	t.Helper()
	root := t.TempDir()
	if err := core.Prepare(root); err != nil {
		t.Fatal(err)
	}
	source := core.Session{ID: strings.Repeat("a", 24), CreateID: "parent", Kind: agent, Account: "work", ProjectID: "project", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID("project", "work", accounts.ProjectLocalOAuth), Workspace: "/workspace", Model: "initial-model", Title: "Same conversation", Supervisor: "/old/supervisor"}
	credential := []byte(`{"claudeAiOauth":{"accessToken":"synthetic"}}`)
	folder, name := "projects", "native-parent.jsonl"
	if agent == "codex" {
		credential = []byte(`{"tokens":{"access_token":"synthetic"}}`)
		folder, name = "sessions", "rollout-native-parent.jsonl"
	}
	profile := accounts.SessionRoot(root, source.CreateID)
	if err := accounts.Install(profile, source.Account, agent, credential); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"child", "trimmed-child"} {
		separate := []byte(`{"claudeAiOauth":{"accessToken":"independent-child"}}`)
		if agent == "codex" {
			separate = []byte(`{"tokens":{"access_token":"independent-child"}}`)
		}
		if err := accounts.Install(accounts.SessionRoot(root, key), source.Account, agent, separate); err != nil {
			t.Fatal(err)
		}
	}
	native := filepath.Join(accounts.Config(profile, source.Account), folder, "workspace", name)
	if err := os.MkdirAll(filepath.Dir(native), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("original native context\n"), 0600); err != nil {
		t.Fatal(err)
	}
	events := []core.Event{
		{Kind: "state", Text: "idle", RunID: "parent-run"},
		{Kind: "vendor", Text: "native-parent"},
		{Kind: "input", Text: "remember the original context"},
		{Kind: "assistant", Text: "I remember", Response: &core.ResponseMetadata{Model: "current-model", Effort: "high", Phase: "final_answer", CompletionJSON: `{"response_seq":"4"}`}},
		{Kind: "receipt", Payload: json.RawMessage(`{"op":"send","command":{"client_id":"old-send"},"status":"accepted"}`)},
		{Kind: "setting", Text: "model", Payload: json.RawMessage(`{"value":"current-model"}`)},
		{Kind: "setting", Text: "effort", Payload: json.RawMessage(`{"value":"high"}`)},
		{Kind: "permission", Text: "ask"},
	}
	for i := range events {
		events[i].Seq = uint64(i + 1)
		events[i].SessionID = source.ID
		events[i].TimeMS = int64(1000 + i)
	}
	return root, source, events
}

func TestForkPreservesConversationButSeparatesRuntimeAndNativeFiles(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			root, source, events := fixture(t, agent)
			snapshot := supervisor.Replay(events)
			child, err := sessionfork.Prepare(t.Context(), root, source, snapshot, events, sessionfork.Request{ID: strings.Repeat("b", 24), ClientID: "child", RunID: snapshot.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if child.ID == source.ID || child.Workspace != source.Workspace || child.Account != source.Account || child.AuthBinding != source.AuthBinding || child.Kind != source.Kind || child.Model != "current-model" || child.Title != source.Title || child.Supervisor != "" {
				t.Fatal("wrong inherited settings", child)
			}
			if child.Fork.SessionID != source.ID || child.Fork.ThroughSeq != snapshot.LastSeq {
				t.Fatal("missing lineage", child.Fork)
			}
			copied, err := childEvents(t, root, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			if copied[3].Text != events[3].Text || copied[3].TimeMS != events[3].TimeMS || copied[3].Response.CompletionJSON != events[3].Response.CompletionJSON || copied[3].Seq != events[3].Seq || copied[3].SessionID != child.ID {
				t.Fatal("historical response changed", copied[3])
			}
			if copied[1].Text != "" {
				t.Fatal("parent vendor ID survived in inherited control state")
			}
			replay := supervisor.Replay(copied)
			if replay.VendorID != "" || replay.RunID != "" || replay.State != "stopped" || replay.PermissionMode != "ask" || len(replay.Pending) != 0 {
				t.Fatal("inherited runtime identity", replay)
			}
			if !strings.HasPrefix(child.Fork.NativePath, accounts.SessionRoot(root, child.CreateID)+string(filepath.Separator)) {
				t.Fatal("native fork reads the parent profile")
			}
			native, err := os.ReadFile(child.Fork.NativePath)
			if err != nil || string(native) != "original native context\n" {
				t.Fatal("missing provider context", err)
			}
			if err := os.WriteFile(child.Fork.NativePath, []byte("child context"), 0600); err != nil {
				t.Fatal(err)
			}
			parentConfig := accounts.Config(accounts.SessionRoot(root, source.CreateID), source.Account)
			relative, _ := filepath.Rel(accounts.Config(accounts.SessionRoot(root, child.CreateID), child.Account), child.Fork.NativePath)
			parentNative, err := os.ReadFile(filepath.Join(parentConfig, relative))
			if err != nil || string(parentNative) != "original native context\n" {
				t.Fatal("child overwrote parent native context", err)
			}
			if _, err := accounts.LaunchSession(root, child, nil); err != nil {
				t.Fatal("same account not available to child", err)
			}
		})
	}
}

func TestForkRejectsUnsettledStaleAndMissingNativeContext(t *testing.T) {
	for _, scenario := range []string{"working", "stale", "question", "queued", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			root, source, events := fixture(t, "claude")
			snapshot := supervisor.Replay(events)
			request := sessionfork.Request{ID: strings.Repeat("b", 24), ClientID: "child", RunID: snapshot.RunID}
			switch scenario {
			case "working":
				snapshot.State = "working"
			case "stale":
				request.RunID = "stale"
			case "question":
				snapshot.Pending = []core.Event{{Kind: "approval"}}
			case "queued":
				snapshot.Queued = "waiting"
			case "missing":
				os.RemoveAll(filepath.Join(accounts.Config(accounts.SessionRoot(root, source.CreateID), source.Account), "projects"))
			}
			if _, err := sessionfork.Prepare(t.Context(), root, source, snapshot, events, request); err == nil {
				t.Fatal("unsafe/incomplete fork accepted")
			}
			if _, err := os.Stat(core.Dir(root, request.ID)); !os.IsNotExist(err) {
				t.Fatal("partial child left behind", err)
			}
		})
	}
}

func TestForkRestoresPreferencesFromATrimmedHistoryCheckpoint(t *testing.T) {
	root, source, events := fixture(t, "codex")
	snapshot := supervisor.Replay(events)
	payload, _ := json.Marshal(core.HistoryCheckpoint{Version: 1, Snapshot: snapshot, Resume: events[5:7]})
	checkpoint := core.Event{Seq: snapshot.LastSeq, Kind: core.HistoryCheckpointKind, Payload: payload}
	child, err := sessionfork.Prepare(t.Context(), root, source, snapshot, []core.Event{checkpoint}, sessionfork.Request{ID: strings.Repeat("c", 24), ClientID: "trimmed-child", RunID: snapshot.RunID})
	if err != nil {
		t.Fatal(err)
	}
	history, err := childEvents(t, root, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range history[1:] {
		if event.Kind == "setting" && event.Text == "effort" && string(event.Payload) == `{"value":"high"}` {
			found = true
		}
	}
	if !found || child.Model != "current-model" || supervisor.Replay(history).VendorID != "" {
		t.Fatal("checkpoint preferences/identity lost", history)
	}
}

// childEvents is the conversation a fork wrote: into the child's own store,
// which is where a conversation lives now. See cxz#278.
func childEvents(t *testing.T, root, id string) ([]core.Event, error) {
	t.Helper()
	store, err := sessionstore.Open(context.Background(), sessionstore.Path(core.Dir(root, id)), id, false)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.Events(context.Background())
}
