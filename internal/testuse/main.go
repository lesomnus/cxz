// Deterministic version-switch fixture; never linked into cxz.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/transport"
	"os"
	"path/filepath"
	"time"
)

type snapshot struct {
	Running, Stopped *api.Session
	PIDs             map[string]int
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := "/cxz/state/data"
	conn, e := transport.Dial(root)
	if e != nil {
		return e
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	for {
		q, cancel := context.WithTimeout(ctx, time.Second)
		_, e = client.List(q, &api.Empty{})
		cancel()
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	path := filepath.Join(root, "fixture.json")
	if os.Args[1] == "init" {
		if e = client.EnsureAccount(ctx, "test", "claude"); e != nil {
			return e
		}
		var sessions []*api.Session
		for _, key := range []string{"running", "stopped"} {
			if e = accounts.Install(accounts.SessionRoot(root, key), "test", "claude", []byte(`{"claudeAiOauth":{"accessToken":"synthetic-test-only"}}`)); e != nil {
				return e
			}
			s, e := client.Create(ctx, &api.CreateRequest{Workspace: "/work", ClientId: key, Account: "test", Model: "fixture-model"})
			if e != nil {
				return e
			}
			sessions = append(sessions, s)
		}
		if _, e = client.Stop(ctx, &api.Control{SessionId: sessions[1].Id, RunId: sessions[1].RunId, ClientId: core.ID()}); e != nil {
			return e
		}
		if _, e = client.Send(ctx, &api.Input{SessionId: sessions[0].Id, RunId: sessions[0].RunId, ClientId: "original-input", Text: "wait"}); e != nil {
			return e
		}
		for {
			sessions[0], e = client.Get(ctx, &api.SessionRef{Id: sessions[0].Id})
			if e != nil {
				return e
			}
			if sessions[0].State == "working" && sessions[0].VendorId != "" {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		var pids map[string]int
		b, e := os.ReadFile(filepath.Join(core.Dir(root, sessions[0].Id), "pid.json"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &pids); e != nil {
			return e
		}
		return core.WriteJSON(path, snapshot{Running: sessions[0], Stopped: sessions[1], PIDs: pids})
	}
	var before snapshot
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &before); e != nil {
		return e
	}
	current, e := client.Get(ctx, &api.SessionRef{Id: before.Running.Id})
	if e != nil {
		return e
	}
	stopped, e := client.Get(ctx, &api.SessionRef{Id: before.Stopped.Id})
	if e != nil {
		return e
	}
	if current.RunId == before.Running.RunId || current.State != "idle" || current.Account != before.Running.Account || current.AuthBinding != before.Running.AuthBinding || current.Model != before.Running.Model || current.VendorId != before.Running.VendorId {
		return fmt.Errorf("session did not resume with its identity preserved: %+v", current)
	}
	if stopped.RunId != before.Stopped.RunId || stopped.State != "stopped" {
		return fmt.Errorf("stopped session was restarted")
	}
	var pids map[string]int
	b, e = os.ReadFile(filepath.Join(core.Dir(root, current.Id), "pid.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &pids); e != nil {
		return e
	}
	if pids["agent"] == before.PIDs["agent"] || pids["supervisor"] == before.PIDs["supervisor"] {
		return fmt.Errorf("agent/supervisor not restarted")
	}
	history, e := client.History(ctx, &api.WatchRequest{SessionId: current.Id})
	if e != nil {
		return e
	}
	count := 0
	for _, event := range history.Events {
		if event.Kind == "input" && event.Text == "wait" {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("original input replayed: %d", count)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"run": current.RunId, "agent": pids["agent"], "supervisor": pids["supervisor"]})
}
