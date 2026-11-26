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
	"github.com/lesomnus/cxz/internal/telemetrystore"
	"github.com/lesomnus/cxz/resource"
)

func (m *Manager) telemetryClient() *telemetrystore.Client { return m.telemetry.Load() }

func (m *Manager) writeTelemetryRegistry(p *Project) error {
	if m.telemetryClient() == nil {
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
	if m.telemetryClient() == nil || in.Session == "" || in.Task == "models" {
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
	store := m.telemetryClient()
	if store == nil {
		return nil, false, nil
	}
	reply, err := store.Stats(ctx, scope, id, from, to)
	return reply, true, err
}

// StartTelemetry schedules installation maintenance after the Manager can serve
// health checks. Docker downloads/schema setup never delay session control.
func (m *Manager) StartTelemetry(ctx context.Context) {
	var config telemetryconfig.Config
	if raw := os.Getenv("CXZ_TELEMETRY_CONFIG"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			fmt.Fprintln(os.Stderr, "telemetry configuration:", err)
			return
		}
	}
	var stack telemetrystack.Stack
	if m.kube != nil {
		// On Kubernetes the database and collector are declared beside the
		// manager; it configures and checks them and runs neither.
		// The server may be shared by many installations, each with its own
		// account and database on it; see docs/kubernetes.md.
		host, httpEndpoint, native := os.Getenv("CXZ_TELEMETRY_DB_HOST"), os.Getenv("CXZ_TELEMETRY_DB_HTTP"), os.Getenv("CXZ_TELEMETRY_DB_NATIVE")
		if m.Owner == "" || (host == "" && (httpEndpoint == "" || native == "")) {
			fmt.Fprintln(os.Stderr, "telemetry: no database is configured (CXZ_TELEMETRY_DB_HOST, or CXZ_TELEMETRY_DB_HTTP and CXZ_TELEMETRY_DB_NATIVE); usage is not collected")
			return
		}
		stack = telemetrystack.Stack{Root: m.Root, Owner: m.Owner, Config: config, External: true, DBHost: host, DBHTTP: httpEndpoint, DBNative: native, DBUser: os.Getenv("CXZ_TELEMETRY_DB_USER"), DBName: os.Getenv("CXZ_TELEMETRY_DB_NAME"), Password: os.Getenv("CXZ_TELEMETRY_PASSWORD"), Listen: os.Getenv("CXZ_TELEMETRY_LISTEN"), Health: os.Getenv("CXZ_TELEMETRY_HEALTH")}
	} else {
		if m.Owner == "" || m.StateVolume == "" {
			return
		}
		if _, err := os.Stat("/usr/local/bin/cxz-otelcol"); err != nil {
			return
		}
		stack = telemetrystack.Stack{Root: m.Root, Owner: m.Owner, StateVolume: m.StateVolume, ToolsVolume: m.ToolsVolume, Image: m.Image, Manager: m.Container, Config: config}
	}
	// Usage has no other source, so a store that could not be reached at
	// startup is retried with the rest of the maintenance rather than given up
	// on until the Manager restarts. The first attempt stays synchronous: its
	// setup only writes local files.
	connect := func() error {
		if m.telemetryClient() != nil {
			return nil
		}
		if err := stack.Prepare(); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		client, err := stack.Client()
		if err != nil {
			return fmt.Errorf("client: %w", err)
		}
		m.telemetry.Store(client)
		// Ingestion is authorized from these registries, which are skipped
		// while there is no client; write every project's now.
		m.writeMu.Lock()
		defer m.writeMu.Unlock()
		projects, err := m.all(ctx)
		if err != nil {
			return fmt.Errorf("registry: %w", err)
		}
		for _, p := range projects {
			if err = m.writeTelemetryRegistry(p); err != nil {
				return fmt.Errorf("registry: %w", err)
			}
		}
		return nil
	}
	if err := connect(); err != nil {
		fmt.Fprintln(os.Stderr, "telemetry", err)
	}
	go func() {
		for {
			work, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err := stack.Ensure(work)
			if err == nil {
				err = connect()
			}
			if err == nil {
				err = m.telemetryClient().CleanErased(work)
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
	store := m.telemetryClient()
	if store == nil {
		return nil
	}
	if session != "" {
		if err := store.Erase(ctx, hex.EncodeToString(resourceid.Bytes(8, session))); err != nil {
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
