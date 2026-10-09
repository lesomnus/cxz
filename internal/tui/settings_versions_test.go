package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/internal/versionpin"
)

func TestSettingsVersionsAndStaleUpstream(t *testing.T) {
	m := conversationModel()
	m.ctx = versionpin.WithClient(m.ctx, t.TempDir())
	m.terminalWidth, m.height = 160, 40
	p := &settingsPage{loaded: true, info: settingsInfo{CXZVersion: "v0.1.2", CXZRevision: strings.Repeat("a", 40), CXZChannel: "stable", CXZPin: "v0.1.2"}}
	m.settingsPage = p
	m.Update(settingsUpstreamResult{p, map[string]string{"edge": "source-bbbbbbbbbbbb", "stable": "v0.1.2"}})
	screen := ansi.Strip(m.settingsScreen())
	for _, want := range []string{"Client:", "@edge", "Connected Manager: v0.1.2 (aaaaaaaaaaaa) · @stable · pinned v0.1.2", "Upstream @edge: source-bbbbbbbbbbbb", "Upstream @stable: v0.1.2"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q in %s", want, screen)
		}
	}
	replacement := &settingsPage{}
	m.settingsPage = replacement
	m.Update(settingsUpstreamResult{p, map[string]string{"edge": "stale"}})
	if replacement.upstream != nil {
		t.Fatal("old connection result replaced new page")
	}
	if !strings.Contains(settingsRemoteVersion(&settingsPage{loaded: true}), "update Manager") {
		t.Fatal("old server claimed a version")
	}
}
