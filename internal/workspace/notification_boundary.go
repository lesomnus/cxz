package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/core"
)

// Fork metadata survives display-history retention and project restarts.
func (m *Manager) NotificationBoundary(ctx context.Context, id string) (uint64, error) {
	if !assets.ValidID(id) {
		return 0, errors.New("invalid session ID")
	}
	p, err := m.sessionProject(ctx, id)
	if err != nil {
		return 0, err
	}
	dir, _ := m.projectState(p.storageID())
	b, err := os.ReadFile(filepath.Join(core.Dir(filepath.Join(dir, "data"), id), "manifest.json"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var session core.Session
	if err := json.Unmarshal(b, &session); err != nil {
		return 0, err
	}
	if session.Fork != nil {
		return session.Fork.ThroughSeq + 1, nil
	}
	return 0, nil
}

func (m *Manager) NotificationHead(ctx context.Context, id string) (uint64, error) {
	store, err := m.conversationStore(ctx, id)
	if errors.Is(err, errNoConversation) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer store.Close()
	var head uint64
	err = store.DB().QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?`, id).Scan(&head)
	return head, err
}
