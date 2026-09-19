package workspace

import (
	"context"
	"crypto/subtle"
	"github.com/lesomnus/cxz/internal/quotashare"
)

func (m *Manager) AuthorizeQuota(ctx context.Context, r quotashare.Request, token string) bool {
	p, err := m.resolve(ctx, r.Project)
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) != 1 {
		return false
	}
	for _, s := range p.Sessions {
		if s.Id == r.Session && s.Account == r.Account && s.Agent == "claude" {
			return true
		}
	}
	return false
}
