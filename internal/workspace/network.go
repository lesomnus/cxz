package workspace

import (
	"context"

	"github.com/lesomnus/cxz/internal/dockerx"
)

// One transport network per installation avoids consuming a Docker address pool
// for every workspace. User-defined Compose application networks remain separate.
func (m *Manager) sharedNetwork() string { return "cxz-" + m.Owner[:12] + "-workspaces" }

func (m *Manager) ensureProjectNetwork(ctx context.Context, p *Project) error {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	project := p.ID
	if p.Network == m.sharedNetwork() {
		project = ""
	}
	return dockerx.EnsureResource(ctx, "network", p.Network, m.Owner, project)
}
