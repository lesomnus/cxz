package workspace

import (
	"context"
	"crypto/subtle"
	"github.com/lesomnus/cxz/internal/conversation"
)

func (m *Manager) AuthorizeConversation(ctx context.Context, q conversation.RegistryRequest, token string) bool {
	p, err := m.resolve(ctx, q.Project)
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) != 1 {
		return false
	}
	for _, s := range p.Sessions {
		if s.Id == q.Caller {
			return true
		}
	}
	return false
}
