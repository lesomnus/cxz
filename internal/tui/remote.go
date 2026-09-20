package tui

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
)

func workspacePath(ctx context.Context, input string) (string, error) {
	if transport.IsRemote(ctx) {
		if !strings.HasPrefix(input, "/") || strings.ContainsAny(input, "\x00\r\n") {
			return "", fmt.Errorf("enter an absolute Linux workspace path on the remote daemon host")
		}
		return path.Clean(input), nil
	}
	return filepath.Abs(input)
}

// A connection is chosen on the UI goroutine and captured before asynchronous
// requests. Overlays retain their target when the panel selection changes.
func (m *model) connectionRef() string {
	if m.accountView {
		return m.accountConnection
	}
	if m.creating {
		return m.creationConnection
	}
	if m.panelFocus || m.projectView {
		rows := m.panelRows()
		if m.panelIndex >= 0 && m.panelIndex < len(rows) {
			return rows[m.panelIndex].project.Id
		}
	}
	if s := m.current(); s != nil {
		return s.Id
	}
	if m.project != nil {
		return m.project.Id
	}
	return ""
}
func (m *model) contextFor(ref string) context.Context {
	if c, ok := m.client.(interface {
		ContextFor(context.Context, string) context.Context
	}); ok {
		return c.ContextFor(m.ctx, ref)
	}
	return m.ctx
}
func (m *model) connectionName(ref string) string {
	if c, ok := m.client.(interface{ ConnectionName(string) string }); ok {
		return c.ConnectionName(ref)
	}
	return ""
}
func (m *model) connectionLabel(ref string) string {
	if name := m.connectionName(ref); name != "" {
		return " via " + name
	}
	return ""
}
func (m *model) accountClient() resource.AccountServiceClient {
	if c, ok := m.client.(interface {
		AccountClient(string) resource.AccountServiceClient
	}); ok {
		return c.AccountClient(m.connectionRef())
	}
	return m.accountService
}
func (m *model) localProject(p *api.Project) *api.Project {
	if c, ok := m.client.(interface {
		LocalProject(*api.Project) *api.Project
	}); ok {
		return c.LocalProject(p)
	}
	return p
}
