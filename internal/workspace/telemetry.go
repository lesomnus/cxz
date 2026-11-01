package workspace

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/internal/telemetry"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/telemetryconfig"
	"github.com/lesomnus/cxz/internal/telemetrystack"
	"github.com/lesomnus/cxz/resource"
)

func (m *Manager) writeTelemetryRegistry(p *Project) error {
	if m.telemetry == nil {
		return nil
	}
	key, err := telemetryauth.ProjectKey(p.ID)
	if err != nil {
		return err
	}
	registry := telemetryauth.Registry{Secret: p.Token, Sessions: map[string]telemetryauth.Session{}}
	for _, s := range p.Sessions {
		if !m.removedSessions[s.Id] {
			registry.Sessions[s.Id] = telemetryauth.Session{Account: s.Account, Agent: s.Agent}
		}
	}
	for id, entry := range m.telemetryAux[p.ID] {
		if !m.removedSessions[entry.Parent] {
			registry.Sessions[id] = entry
		}
	}
	return core.WriteJSON(filepath.Join(m.Root, "telemetry", "auth", key+".json"), registry)
}

// Auxiliary tasks use their actual configured account and a short-lived ingest
// registration, while statistics remain attributable to the parent session.
func (m *Manager) auxiliaryTelemetry(ctx context.Context, in auxiliary.Input) (*telemetry.Publisher, func()) {
	if m.telemetry == nil || in.Session == "" || in.Task == "models" {
		return nil, func() {}
	}
	p, err := m.projectOf(ctx, in.Session)
	if err != nil {
		return nil, func() {}
	}
	identity := telemetryauth.Identity{Project: p.ID, Session: "auxiliary-" + core.ID(), Parent: in.Session, Account: in.Profile.Account, Agent: in.Profile.Agent, Run: core.ID(), Source: "supervisor", UsageSource: "supervisor", Scope: "auxiliary"}
	m.writeMu.Lock()
	if m.telemetryAux == nil {
		m.telemetryAux = map[string]map[string]telemetryauth.Session{}
	}
	if m.telemetryAux[p.ID] == nil {
		m.telemetryAux[p.ID] = map[string]telemetryauth.Session{}
	}
	m.telemetryAux[p.ID][identity.Session] = telemetryauth.Session{Account: identity.Account, Agent: identity.Agent, Scope: identity.Scope, Parent: identity.Parent}
	err = m.writeTelemetryRegistry(p)
	m.writeMu.Unlock()
	cleanup := func() {
		m.writeMu.Lock()
		defer m.writeMu.Unlock()
		delete(m.telemetryAux[p.ID], identity.Session)
		// Use current state rather than the project's pre-execution snapshot.
		if current, err := m.resolve(context.Background(), p.ID); err == nil {
			if err = m.writeTelemetryRegistry(current); err != nil {
				fmt.Fprintln(os.Stderr, "telemetry registry:", err)
			}
		}
	}
	if err != nil {
		cleanup()
		return nil, func() {}
	}
	publisher, err := telemetry.NewPublisher(telemetryauth.Socket, p.Token, identity)
	if err != nil {
		cleanup()
		return nil, func() {}
	}
	return publisher, func() { publisher.Close(); cleanup() }
}

func (m *Manager) TelemetryStats(ctx context.Context, scope, id string, from, to int64) (*resource.ConversationStatsReply, bool, error) {
	if m.telemetry == nil {
		return nil, false, nil
	}
	reply, err := m.telemetry.Stats(ctx, scope, id, from, to)
	return reply, true, err
}

// StartTelemetry schedules installation maintenance after the Manager can serve
// health checks. Docker downloads/schema setup never delay session control.
func (m *Manager) StartTelemetry(ctx context.Context) {
	if m.Owner == "" || m.StateVolume == "" {
		return
	}
	if _, err := os.Stat("/usr/local/bin/cxz-otelcol"); err != nil {
		return
	}
	var config telemetryconfig.Config
	if raw := os.Getenv("CXZ_TELEMETRY_CONFIG"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			fmt.Fprintln(os.Stderr, "telemetry configuration:", err)
			return
		}
	}
	stack := telemetrystack.Stack{Root: m.Root, Owner: m.Owner, StateVolume: m.StateVolume, ToolsVolume: m.ToolsVolume, Image: m.Image, Manager: m.Container, Config: config}
	if config.Active() {
		if err := stack.Prepare(); err != nil {
			fmt.Fprintln(os.Stderr, "telemetry setup:", err)
			return
		}
		var err error
		m.telemetry, err = stack.Client()
		if err != nil {
			fmt.Fprintln(os.Stderr, "telemetry client:", err)
			return
		}
		m.writeMu.Lock()
		projects, err := m.all(ctx)
		if err == nil {
			for _, p := range projects {
				if e := m.writeTelemetryRegistry(p); e != nil {
					err = e
					break
				}
			}
		}
		m.writeMu.Unlock()
		if err != nil {
			fmt.Fprintln(os.Stderr, "telemetry registry:", err)
		}
	}
	go func() {
		for {
			work, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err := stack.Ensure(work)
			if err == nil && m.telemetry != nil {
				err = m.telemetry.CleanErased(work)
			}
			cancel()
			if err != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, "telemetry maintenance:", err)
			}
			pause := 5 * time.Minute
			if err != nil {
				pause = 30 * time.Second
			}
			timer := time.NewTimer(pause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (m *Manager) eraseTelemetry(ctx context.Context, project, session string) error {
	if m.telemetry == nil {
		return nil
	}
	if session != "" {
		if err := m.telemetry.Erase(ctx, hex.EncodeToString(resourceid.Bytes(8, session))); err != nil {
			return err
		}
	} else if project != "" {
		// Project erasure supplies every known session separately. Registry
		// revocation also rejects exports from an old detached process.
		key, err := telemetryauth.ProjectKey(project)
		if err != nil {
			return err
		}
		if err = os.Remove(filepath.Join(m.Root, "telemetry", "auth", key+".json")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
