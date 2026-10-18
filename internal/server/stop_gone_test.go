package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

// supervisorRefusingStop listens where a session's supervisor would and refuses
// to stop, so that a refusal can be told apart from an absence.
func supervisorRefusingStop(t *testing.T, root, id string) {
	t.Helper()
	sock := core.Socket(root, id)
	if err := os.MkdirAll(filepath.Dir(sock), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stop") {
			http.Error(w, "refused by this supervisor", http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(core.Snapshot{State: "idle", RunID: "run"})
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// Stopping a session whose supervisor is gone used to be impossible: the stop
// went through the socket that was not there, and the session stayed in a state
// that also blocked every update. See cxz#223.
func TestStoppingASessionWithNoSupervisorSucceeds(t *testing.T) {
	s, m, l := projectionFixture(t)
	if _, err := l.AppendBatch([]core.Event{{SessionID: m.ID, Kind: "state", Text: "idle", RunID: "run"}}); err != nil {
		t.Fatal(err)
	}
	v, err := s.Stop(context.Background(), &api.Control{SessionId: m.ID, RunId: "run", ClientId: "client"})
	if err != nil {
		t.Fatal("a session with no supervisor could not be stopped:", err)
	}
	if v.Status != "stopped" || v.ClientId != "client" {
		t.Fatal("unexpected receipt", v)
	}
}

// A supervisor that is listening still decides for itself, so its refusal is an
// error rather than a silent success.
func TestStoppingASessionWhoseSupervisorRefusesIsAnError(t *testing.T) {
	s, m, l := projectionFixture(t)
	if _, err := l.AppendBatch([]core.Event{{SessionID: m.ID, Kind: "state", Text: "idle", RunID: "run"}}); err != nil {
		t.Fatal(err)
	}
	supervisorRefusingStop(t, s.root, m.ID)
	_, err := s.Stop(context.Background(), &api.Control{SessionId: m.ID, RunId: "run", ClientId: "client"})
	if err == nil {
		t.Fatal("a refused stop was reported as success")
	}
	if !strings.Contains(err.Error(), "refused by this supervisor") {
		t.Fatal("the refusal did not travel:", err)
	}
}
