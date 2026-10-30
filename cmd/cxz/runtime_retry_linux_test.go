package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
)

func TestServerStatusIncludesPersistentRuntimeRecovery(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CXZ_MANAGER_CONTAINER", "")
	t.Setenv("CXZ_OWNER", "")
	path := filepath.Join(root, "projects", "p", "runtime-recovery.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	r := cxzupdate.Recovery{Project: "p", Container: "container", Transaction: "tx", Phase: "rolling_back", Status: "retrying", Attempts: 2, MaxAttempts: 5, NextAttempt: time.Now().Add(time.Minute)}
	if err := core.WriteJSON(path, r); err != nil {
		t.Fatal(err)
	}
	b, err := localUpdateCommand(t.Context(), root, "status")
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Recoveries []cxzupdate.Recovery `json:"runtime_recoveries"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Recoveries) != 1 || out.Recoveries[0].Project != "p" || out.Recoveries[0].Attempts != 2 || out.Recoveries[0].Phase != "rolling_back" || !out.Recoveries[0].NextAttempt.Equal(r.NextAttempt) {
		t.Fatalf("status lost recovery scheduling metadata: %s", b)
	}
	// Status is observational: reading it cannot reset or consume the budget.
	after, err := cxzupdate.LoadRecovery(path)
	if err != nil || after.Attempts != r.Attempts || after.Status != r.Status {
		t.Fatalf("status changed retry policy: %+v, %v", after, err)
	}
}
