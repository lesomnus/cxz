package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

// Models answers from the projection the runtime already maintains, so a client
// asking "what can this agent run" costs one request instead of a walk through
// the journal. refresh additionally asks the agent; its answer is published as a
// journal event, which a client watching the session receives without asking
// again -- so nothing here waits on a provider.
func (s *Server) Models(ctx context.Context, r *api.ModelsRequest) (*api.ModelsReply, error) {
	if s.manager != nil {
		c, client, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		return client.Models(ctx, r)
	}
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	out := &api.ModelsReply{}
	if r.Refresh {
		// A refusal here is not a failure to report: the catalog that is already
		// known is still the best answer, and the reason belongs beside it.
		if _, e := s.command(ctx, r.SessionId, "models", core.Command{RunID: r.RunId, ClientID: r.ClientId}); e != nil {
			out.Status = e.Error()
		} else {
			out.Refreshing = true
		}
	}
	p, err := s.lockProjection(ctx, m)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	out.LastSeq, out.RunId = p.cursor.Seq, p.snapshot.RunID
	if v, ok := p.catalogs[p.snapshot.RunID]; ok {
		out.Data, out.CatalogSeq, out.CatalogMs = v.payload, v.seq, v.ms
	}
	return out, nil
}
