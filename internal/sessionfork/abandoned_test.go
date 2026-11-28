package sessionfork_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionfork"
	"github.com/lesomnus/cxz/internal/supervisor"
)

// A fork that dies before it writes session.json leaves a directory named
// after a child that does not exist. The identity is derived from the source
// and the client id, so the retry asks for that same name and used to fail on
// it -- forever, until somebody deleted the directory by hand. See cxz#396.
func TestForkRetriesOverAnUnfinishedAttempt(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root, source, events := fixture(t, agent)
			snapshot := supervisor.Replay(events)
			request := sessionfork.Request{ID: strings.Repeat("b", 24), ClientID: "child", RunID: snapshot.RunID}

			// What an interrupted attempt leaves: the child's directory with
			// no session.json, and a half-copied provider history.
			dir := core.Dir(root, request.ID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "conversation.db"), []byte("torn"), 0600); err != nil {
				t.Fatal(err)
			}
			profile := accounts.SessionRoot(root, request.ClientID)
			folder := "projects"
			if agent == "codex" {
				folder = "sessions"
			}
			history := filepath.Join(accounts.Config(profile, source.Account), folder, "workspace")
			if err := os.MkdirAll(history, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(history, "half.jsonl"), []byte("half"), 0600); err != nil {
				t.Fatal(err)
			}

			child, err := sessionfork.Prepare(t.Context(), root, source, snapshot, events, request)
			if err != nil {
				t.Fatal(err)
			}
			if child.ID != request.ID || child.Fork == nil {
				t.Fatalf("%+v", child)
			}
			if _, err = os.Stat(filepath.Join(core.Dir(root, child.ID), "session.json")); err != nil {
				t.Fatal("the retry did not finish:", err)
			}
			// The debris is gone rather than merged into the child.
			if _, err = os.Stat(filepath.Join(history, "half.jsonl")); !os.IsNotExist(err) {
				t.Fatal("the unfinished copy survived into the child:", err)
			}
		})
	}
}

// A finished fork is never cleared. The caller answers from session.json
// instead of forking again, and a second Prepare for the same identity must
// not take the child apart.
func TestForkLeavesAFinishedChildAlone(t *testing.T) {
	root, source, events := fixture(t, "claude")
	snapshot := supervisor.Replay(events)
	request := sessionfork.Request{ID: strings.Repeat("b", 24), ClientID: "child", RunID: snapshot.RunID}
	child, err := sessionfork.Prepare(t.Context(), root, source, snapshot, events, request)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(core.Dir(root, child.ID), "conversation.db")
	before, err := os.Stat(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessionfork.Prepare(t.Context(), root, source, snapshot, events, request); err == nil {
		t.Fatal("forking over a finished child was allowed")
	}
	after, err := os.Stat(store)
	if err != nil {
		t.Fatal("the finished child's conversation was removed:", err)
	}
	if before.Size() != after.Size() {
		t.Fatalf("the finished child's conversation changed: %d -> %d", before.Size(), after.Size())
	}
}
