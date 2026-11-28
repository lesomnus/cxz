package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

// The point of the split is that the three parts answer to different owners: a
// retention policy can reach the journal, nothing in cxz prunes the provider's
// transcripts, and the rest is the agent's own clutter. Reporting one number
// would not say which of those to act on.
func TestSessionStorageSeparatesJournalProviderAndProfile(t *testing.T) {
	root := t.TempDir()
	id, create := "0123456789abcdef01234567", "creation-key"
	write := func(path string, size int) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0600); err != nil {
			t.Fatal(err)
		}
	}
	session := core.Session{ID: id, CreateID: create, Kind: "claude", Account: "work1"}
	if err := os.MkdirAll(core.Dir(root, id), 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(core.Dir(root, id), "session.json"), session); err != nil {
		t.Fatal(err)
	}
	manifest, err := core.TreeBytes(context.Background(), core.Dir(root, id))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(core.Dir(root, id), "conversation.db"), 4096)
	write(filepath.Join(core.Dir(root, id), "conversation.db-wal"), 1024)

	profile := accounts.SessionRoot(root, create)
	config := accounts.Config(profile, "work1")
	write(filepath.Join(config, "projects", "-workspace", "vendor.jsonl"), 9000)
	write(filepath.Join(config, "projects", "-workspace", "vendor", "subagents", "a.jsonl"), 1000)
	write(filepath.Join(profile, "home", ".cache", "blob"), 7000)
	write(filepath.Join(profile, "tmp", "scratch"), 500)

	out, err := sessionStorage(context.Background(), root, id)
	if err != nil {
		t.Fatal(err)
	}
	if out.JournalBytes != 4096+1024+manifest {
		t.Fatal("journal bytes", out.JournalBytes)
	}
	if out.ProviderBytes != 9000+1000 {
		t.Fatal("provider bytes", out.ProviderBytes)
	}
	if out.ProfileBytes != 7000+500 {
		t.Fatal("profile bytes should exclude the provider's transcripts", out.ProfileBytes)
	}
	if out.TotalBytes != out.JournalBytes+out.ProviderBytes+out.ProfileBytes {
		t.Fatal("total is not the sum", out.TotalBytes)
	}
}

// A session can be asked about before it has written anything, and before it
// has a profile at all; that is zero rather than a refusal.
func TestSessionStorageOfASessionThatHasWrittenNothing(t *testing.T) {
	root := t.TempDir()
	out, err := sessionStorage(context.Background(), root, "0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if out.TotalBytes != 0 || out.JournalBytes != 0 || out.ProviderBytes != 0 || out.ProfileBytes != 0 {
		t.Fatal("expected zeroes", out)
	}
	if _, err := sessionStorage(context.Background(), root, "../escape"); err == nil {
		t.Fatal("an invalid session id was accepted")
	}
}

func TestSessionStorageUsesTheCodexHistoryFolder(t *testing.T) {
	root := t.TempDir()
	id, create := "0123456789abcdef01234568", "codex-creation"
	session := core.Session{ID: id, CreateID: create, Kind: "codex", Account: "work2"}
	if err := os.MkdirAll(core.Dir(root, id), 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(core.Dir(root, id), "session.json"), session); err != nil {
		t.Fatal(err)
	}
	config := accounts.Config(accounts.SessionRoot(root, create), "work2")
	path := filepath.Join(config, "sessions", "2026", "10", "05", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 8888), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := sessionStorage(context.Background(), root, id)
	if err != nil {
		t.Fatal(err)
	}
	if out.ProviderBytes != 8888 {
		t.Fatal("codex rollouts were not counted as provider history", out.ProviderBytes)
	}
}
