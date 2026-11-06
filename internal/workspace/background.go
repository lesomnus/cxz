package workspace

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/lesomnus/cxz/api"
)

func (m *Manager) Background(ctx context.Context, r *api.SessionRef) (*api.BackgroundReply, error) {
	store, err := m.conversationStore(ctx, r.Id)
	if errors.Is(err, errNoConversation) {
		return &api.BackgroundReply{Data: []byte(`{}`)}, nil
	}
	if err != nil {
		return nil, err
	}
	defer store.Close()
	// Read the saved fence first: newly appended records may not have been
	// reduced yet, so the events table's highest sequence is not a safe fence.
	snapshot, _, err := store.State(ctx)
	if err != nil {
		return nil, err
	}
	background, err := store.Background(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(background)
	return &api.BackgroundReply{LastSeq: snapshot.LastSeq, Data: data}, err
}
