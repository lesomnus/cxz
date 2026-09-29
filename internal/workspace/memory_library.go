package workspace

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/memorylib"
	"strings"
)

func (m *Manager) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	var out memorylib.Reply
	projects, e := m.all(ctx)
	if e != nil {
		return out, e
	}
	p, s, e := findMemorySession(projects, id)
	if e != nil {
		return out, e
	}
	session := memorySession(s)
	session.ProjectID = p.ID
	session.Title = s.Title
	data, e := m.memoryHelper(ctx, p.ID, "_memory-library", memorylib.HelperRequest{Session: session, Request: q}, []memoryMount{{volume: p.Volume, project: p.ID, target: "/cxz/state", write: true}})
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal(data, &out); e != nil {
		return out, e
	}
	out.Location = "volume:" + p.Volume + strings.TrimPrefix(out.Location, "/cxz/state")
	return out, nil
}
