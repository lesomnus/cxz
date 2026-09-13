package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

func TestAgentUpdatePaths(t *testing.T) {
	if !validAgentUpdate("claude", "/cxz/tools/claude/2.1.300/linux-x64/claude") {
		t.Fatal("release rejected")
	}
	for _, path := range []string{"/tmp/claude", "/cxz/tools/codex/1.0.0/bin/codex", "/cxz/tools/claude/2.1.300/../../claude", "/cxz/tools/claude/latest/linux-x64/claude"} {
		if validAgentUpdate("claude", path) {
			t.Fatal("unsafe release path", path)
		}
	}
}

func TestAgentUpdateCompletionAndRollback(t *testing.T) {
	for _, mode := range []string{"success", "rollback", "rollback_failed"} {
		t.Run(mode, func(t *testing.T) {
			old := core.Session{ID: "s", CreateID: "stable", Agent: "old", Account: "company", AuthBinding: "binding", Model: "model", Workspace: "workspace"}
			path := filepath.Join(t.TempDir(), "agent-update.json")
			if err := core.WriteJSON(path, agentUpdateTransaction{Old: old, Target: "new"}); err != nil {
				t.Fatal(err)
			}
			var saved, launched []core.Session
			stops := 0
			a := agentUpdateActions{
				release: func(ctx context.Context, m core.Session) error { return ctx.Err() },
				save:    func(ctx context.Context, m core.Session) error { saved = append(saved, m); return ctx.Err() },
				launch: func(ctx context.Context, m core.Session) (*api.Session, error) {
					launched = append(launched, m)
					state := "idle"
					if mode == "rollback_failed" || (mode == "rollback" && m.Agent == "new") {
						state = "failed"
					}
					return &api.Session{RunId: "new-run", State: state}, ctx.Err()
				},
				stop:   func(context.Context, string) { stops++ },
				notice: func(context.Context, string, string, string) {},
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			out, err := finishAgentUpdate(ctx, old, "new", path, a)
			if mode == "rollback_failed" {
				if err == nil {
					t.Fatal("failed old agent counted as restored")
				}
				b, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				var tx agentUpdateTransaction
				if json.Unmarshal(b, &tx) != nil || tx.RecoveryAt.IsZero() || tx.Old != old {
					t.Fatal("durable recovery/backoff lost")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, e := os.Stat(path); !os.IsNotExist(e) {
					t.Fatal("completed transaction retained")
				}
				want := "updated"
				if mode == "rollback" {
					want = "rolled_back"
				}
				if out.State != want {
					t.Fatal(out)
				}
			}
			updated := old
			updated.Agent = "new"
			want := []core.Session{updated}
			if mode != "success" {
				want = append(want, old)
				if stops != 1 {
					t.Fatal("failed process not stopped")
				}
			}
			if !reflect.DeepEqual(saved, want) || !reflect.DeepEqual(launched, want) {
				t.Fatal("session/account metadata changed", saved, launched)
			}
		})
	}
}
