package workspace

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionpurge"
)

// PurgeSession destroys a session everywhere cxz keeps it. The project runtime
// goes first because the conversation is the part nothing can reconstruct; the
// manager's own leftovers are small and survive a re-run, so a failure between
// the two loses the journal and nothing else.
func (m *Manager) PurgeSession(ctx context.Context, in *api.SessionPurgeInput) (*api.SessionPurgeReply, error) {
	r := sessionpurge.Request{Session: in.SessionId, DryRun: in.DryRun}
	project, err := m.projectOf(ctx, r.Session)
	if err != nil {
		return nil, err
	}
	conn, client, err := m.client(ctx, project)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	reply, err := client.PurgeSession(ctx, in)
	if err != nil {
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
	for _, t := range here.Targets {
		reply.Targets = append(reply.Targets, &api.SessionPurgeTarget{
			Kind: t.Kind, Path: t.Path, Files: int32(t.Files), Bytes: t.Bytes,
		})
	}
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
	return reply, nil
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
