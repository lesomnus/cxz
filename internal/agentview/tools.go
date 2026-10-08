package agentview

import (
	"github.com/lesomnus/cxz/internal/shellview"
	"github.com/lesomnus/cxz/internal/toolview"
)

// Keep TUI callers on the shared display-only parser used by transcript history.
type ToolActivity = toolview.ToolActivity
type FileActivity = toolview.FileActivity

func ToolView(provider, name string, raw []byte) (ToolActivity, bool) {
	return toolview.ToolView(provider, name, raw)
}
func unwrapShell(command string) (string, string) { return shellview.Unwrap(command) }
