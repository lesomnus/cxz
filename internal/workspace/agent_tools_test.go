package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/projectid"
)

func agentProject(t *testing.T) (*Manager, *Project, Runtime) {
	t.Helper()
	m := &Manager{Root: t.TempDir()}
	p := &Project{ID: projectid.New(), Workspace: "/host/project", RemoteWorkspace: "/workspace/project", Token: "project-token", ContainerID: "container", RemoteUser: "developer"}
	dir, _ := m.projectState(p.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	r := Runtime{ProjectID: p.ID, LegacyProjectID: "legacy", Workspace: p.RemoteWorkspace, Token: p.Token, Claude: "/cxz/tools/claude/2.1.267/linux-x64/claude"}
	if err := m.writeRuntime(t.Context(), p, r); err != nil {
		t.Fatal(err)
	}
	return m, p, r
}

func TestAgentPreparationRepairsMissingAndStalePaths(t *testing.T) {
	for _, stale := range []string{"", "/cxz/tools/codex/0.1.0/removed/bin/codex"} {
		t.Run(stale, func(t *testing.T) {
			m, p, before := agentProject(t)
			before.Codex = stale
			if err := m.writeRuntime(t.Context(), p, before); err != nil {
				t.Fatal(err)
			}
			selected := "/cxz/tools/codex/0.154.0/x86_64-unknown-linux-musl/bin/codex"
			var checked []string
			a := agentPreparation{
				ensure: func(_ context.Context, kind string) (string, error) {
					if kind != "codex" {
						t.Fatal("prepared the wrong provider", kind)
					}
					return selected, nil
				},
				verify: func(_ context.Context, binary string) error {
					checked = append(checked, binary)
					return nil
				},
			}
			for range 2 {
				if err := m.prepareAgentWith(t.Context(), p, "codex", a); err != nil {
					t.Fatal(err)
				}
			}
			got, err := m.projectRuntime(t.Context(), p)
			want := before
			want.Codex = selected
			if err != nil || got != want || !reflect.DeepEqual(checked, []string{selected, selected}) {
				t.Fatal("preparation changed identity, the other provider, or skipped verification", got, checked, err)
			}
			if p.Job.Attempt != 0 || p.Job.State != "" {
				t.Fatal("agent preparation repeated container provisioning", p.Job)
			}
		})
	}
}

func TestAgentPreparationFailuresPreserveRuntime(t *testing.T) {
	for _, failure := range []string{"download", "execution", "canceled", "identity", "invalid manifest", "unknown provider"} {
		t.Run(failure, func(t *testing.T) {
			m, p, _ := agentProject(t)
			dir, _ := m.projectState(p.ID)
			file := filepath.Join(dir, "runtime.json")
			if failure == "identity" {
				p.Token = "different-token"
			}
			if failure == "invalid manifest" {
				if err := os.WriteFile(file, []byte("invalid json"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			a := agentPreparation{
				ensure: func(context.Context, string) (string, error) {
					if failure == "download" {
						return "", errors.New("release offline")
					}
					return "/new/codex", nil
				},
				verify: func(context.Context, string) error { return errors.New("cannot execute CLI") },
			}
			kind := "codex"
			if failure == "unknown provider" {
				kind = "unknown"
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			if err = m.prepareAgentWith(ctx, p, kind, a); err == nil {
				t.Fatal("a failed preparation succeeded")
			}
			after, err := os.ReadFile(file)
			if err != nil || string(after) != string(before) {
				t.Fatal("failed preparation changed the runtime", err)
			}
		})
	}
}

func TestAgentPreparationCanUseAWorkingRegisteredReleaseOffline(t *testing.T) {
	m, p, before := agentProject(t)
	a := agentPreparation{
		ensure: func(context.Context, string) (string, error) { return "", errors.New("offline") },
		verify: func(_ context.Context, binary string) error {
			if binary != before.Claude {
				t.Fatal("did not check the registered executable", binary)
			}
			return nil
		},
	}
	if err := m.prepareAgentWith(t.Context(), p, "claude", a); err != nil {
		t.Fatal("offline release lookup stranded a working project", err)
	}
	after, err := m.projectRuntime(t.Context(), p)
	if err != nil || after != before {
		t.Fatal("fallback changed the registered choice", after, err)
	}
}

func TestConcurrentAgentPreparationPreservesBothProviders(t *testing.T) {
	m, p, before := agentProject(t)
	var wg sync.WaitGroup
	for _, kind := range []string{"claude", "codex"} {
		wg.Go(func() {
			// Creation, login and start all hold this same project lock.
			lock := m.projectLock(p.ID)
			lock.Lock()
			defer lock.Unlock()
			a := agentPreparation{
				ensure: func(_ context.Context, kind string) (string, error) {
					return distribution.AgentPath("/cxz/tools", kind, "amd64", false, "3.0.0")
				},
				verify: func(context.Context, string) error { return nil },
			}
			if err := m.prepareAgentWith(t.Context(), p, kind, a); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	after, err := m.projectRuntime(t.Context(), p)
	want := before
	want.Claude, _ = distribution.AgentPath("/cxz/tools", "claude", "amd64", false, "3.0.0")
	want.Codex, _ = distribution.AgentPath("/cxz/tools", "codex", "amd64", false, "3.0.0")
	if err != nil || after != want {
		t.Fatal("concurrent preparation lost another provider", after, err)
	}
}

func TestOnlySuccessfulRolloutsPromoteAgentDefaults(t *testing.T) {
	for _, state := range []string{"updated", "rolled_back", "waiting", "failed"} {
		t.Run(state, func(t *testing.T) {
			m, p, before := agentProject(t)
			binary := "/cxz/tools/claude/3.0.0/linux-x64/claude"
			if err := m.promoteAgentDefault(t.Context(), p, "claude", &api.AgentUpdateStatus{State: state, Binary: binary}); err != nil {
				t.Fatal(err)
			}
			want := before
			if state == "updated" {
				want.Claude = binary
			}
			got, err := m.projectRuntime(t.Context(), p)
			if err != nil || got != want {
				t.Fatal("rollout promoted a failed release or changed another field", got, err)
			}
			// A slower session must not undo a newer default selected elsewhere.
			if state == "updated" {
				if err = m.promoteAgentDefault(t.Context(), p, "claude", &api.AgentUpdateStatus{State: "updated", Binary: before.Claude}); err != nil {
					t.Fatal(err)
				}
				got, err = m.projectRuntime(t.Context(), p)
				if err != nil || got != want {
					t.Fatal("a stale rollout downgraded the default", got, err)
				}
			}
		})
	}
}
