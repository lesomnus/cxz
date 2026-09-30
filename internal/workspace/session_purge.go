package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionpurge"
)

// PurgeSession destroys a session everywhere cxz keeps it. The project runtime
// goes first because the conversation is the part nothing can reconstruct; the
// manager's own leftovers are small and survive a re-run, so a failure between
// the two loses the journal and nothing else.
func (m *Manager) PurgeSession(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r sessionpurge.Request
	if len(spec) > sessionpurge.MaxSpec || json.Unmarshal(spec, &r) != nil {
		return nil, fmt.Errorf("invalid session purge request")
	}
	project, err := m.projectOf(ctx, r.Session)
	if err != nil {
		return nil, err
	}
	conn, client, err := m.client(ctx, project)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out, err := client.Docker(ctx, &api.DockerInput{Action: "session-purge", Spec: spec})
	if err != nil {
		return nil, err
	}
	var reply sessionpurge.Reply
	if err = json.Unmarshal([]byte(out.Status), &reply); err != nil {
		return nil, err
	}
	subject := sessionpurge.Subject{Session: r.Session, Project: project.ID}
	local := sessionpurge.Execute
	if r.DryRun {
		local = sessionpurge.Plan
	}
	here, err := local(ctx, sessionpurge.Manager, m.Root, subject)
	if err != nil {
		return nil, err
	}
	reply.Targets = append(reply.Targets, here.Targets...)
	if !r.DryRun {
		c, err := m.auxiliaryController()
		if err != nil {
			return nil, err
		}
		if err = c.Purge(r.Session); err != nil {
			return nil, err
		}
	}
	// Say what survives, so a caller reading the reply is not left to assume the
	// session was scrubbed out of places purge has no authority over.
	reply.Retained = []string{
		"project workspace files the agent wrote",
		"attachment bytes other sessions still reference",
		"manager and project logs that mention the session id",
	}
	b, err := json.Marshal(reply)
	return &api.Receipt{Status: string(b)}, err
}

func (m *Manager) projectOf(ctx context.Context, session string) (*Project, error) {
	all, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		for _, s := range p.Sessions {
			if s.Id == session {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("session not found: %s", session)
}
