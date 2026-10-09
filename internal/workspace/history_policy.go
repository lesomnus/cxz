package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) syncHistoryPolicy(ctx context.Context, client api.SessionsClient) error {
	if _, err := os.Stat(filepath.Join(m.Root, "history-policy.json")); os.IsNotExist(err) {
		return nil
	}
	p, err := historypolicy.Load(m.Root)
	if err != nil {
		return err
	}
	_, err = client.SetHistoryPolicy(ctx, historyPolicy(p))
	return err
}

// The policy travels as itself. Zero in a budget still means "use the default",
// which is the policy's own rule rather than the wire's.
func historyPolicy(p historypolicy.Policy) *api.HistoryPolicy {
	return &api.HistoryPolicy{
		Disabled: p.Disabled, MaxMib: int32(p.MaxMiB), RawMib: int32(p.RawMiB),
		WindowMib: int32(p.WindowMiB), WindowTurns: int32(p.WindowTurns),
	}
}

func historyPolicyOf(v *api.HistoryPolicy) historypolicy.Policy {
	if v == nil {
		return historypolicy.Policy{}
	}
	return historypolicy.Policy{
		Disabled: v.Disabled, MaxMiB: int(v.MaxMib), RawMiB: int(v.RawMib),
		WindowMiB: int(v.WindowMib), WindowTurns: int(v.WindowTurns),
	}
}
func (m *Manager) GetHistoryPolicy(ctx context.Context, _ *api.Empty) (*api.HistoryPolicy, error) {
	p, err := historypolicy.Load(m.Root)
	if err != nil {
		return nil, err
	}
	return historyPolicy(p), nil
}

// SetHistoryPolicy saves the budgets and pushes them to the projects that are
// running. A project that cannot take them is named rather than silently left
// on the old ones.
func (m *Manager) SetHistoryPolicy(ctx context.Context, in *api.HistoryPolicy) (*api.HistoryPolicy, error) {
	{
		p := historyPolicyOf(in)
		if err := historypolicy.Save(m.Root, p); err != nil {
			return nil, err
		}
		projects, err := m.all(ctx)
		if err != nil {
			return nil, err
		}
		var failures []error
		for _, p := range projects {
			if p.ContainerID == "" {
				continue
			}
			v, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
			if err == nil && !v.State.Running {
				continue
			}
			if err == nil {
				conn, client, e := m.client(ctx, p)
				err = e
				if err == nil {
					err = m.syncHistoryPolicy(ctx, client)
					conn.Close()
				}
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", p.Name, err))
			}
		}
		if err := errors.Join(failures...); err != nil {
			return nil, fmt.Errorf("history policy saved; some projects require a retry: %w", err)
		}
	}
	return m.GetHistoryPolicy(ctx, &api.Empty{})
}

// A project runtime older than the manager routes an action it does not know
// to the manager path it never has, and answers with that failed precondition;
// one older still does not serve the call at all.
func unsupportedByRuntime(err error) bool {
	switch status.Code(err) {
	case codes.FailedPrecondition, codes.Unimplemented:
		return true
	}
	return false
}

// Every preference here lands at the next agent start rather than in the
// running container, so a runtime that cannot take one must not decide whether
// the project can be used. Refusing took the whole project down -- no resume,
// no new session -- and project runtimes only update once their sessions fall
// quiet, which a stopped session waiting to be resumed cannot bring about.
func (m *Manager) syncRuntimePreferences(ctx context.Context, client api.SessionsClient, project string) error {
	for _, push := range []func() error{
		func() error { return m.syncFileMappings(ctx, client) },
		func() error { return m.syncHistoryPolicy(ctx, client) },
		func() error { return m.syncMCP(ctx, client, project) },
		func() error { return m.syncSkills(ctx, client, project) },
	} {
		if err := push(); err != nil && !unsupportedByRuntime(err) {
			return err
		}
	}
	return nil
}
