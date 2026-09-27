package tui

import (
	"context"
	"github.com/lesomnus/cxz/internal/versionpin"
	"testing"
)

func TestPinRestartWaitsForCompleteSwitch(t *testing.T) {
	root := t.TempDir()
	m := model{ctx: versionpin.WithClient(context.Background(), root), pinGeneration: "old"}
	p := versionpin.Pin{Version: "v0.1.0", Generation: "new"}
	if e := versionpin.Save(root, p); e != nil {
		t.Fatal(e)
	}
	if m.pinnedRestart() != nil {
		t.Fatal("restarted while server switch pending")
	}
	m.pinChecked = m.pinChecked.Add(-2e9)
	p.Ready = true
	if e := versionpin.Save(root, p); e != nil {
		t.Fatal(e)
	}
	if m.pinnedRestart() == nil || m.pinRestart == nil {
		t.Fatal("completed switch not observed")
	}
	m.pinGeneration = "new"
	m.pinChecked = m.pinChecked.Add(-2e9)
	if m.pinnedRestart() != nil {
		t.Fatal("same generation restarted repeatedly")
	}
}

func TestSettingsShowsChannelOrFixedVersion(t *testing.T) {
	root := t.TempDir()
	m := model{ctx: versionpin.WithClient(context.Background(), root)}
	if got := m.releaseSelectionText(); got != "cxz update channel: @edge" {
		t.Fatal(got)
	}
	if e := versionpin.Save(root, versionpin.Pin{Version: "v0.1.2", Channel: "stable", Ready: true}); e != nil {
		t.Fatal(e)
	}
	if got := m.releaseSelectionText(); got != "cxz update channel: @stable" {
		t.Fatal(got)
	}
	if e := versionpin.Save(root, versionpin.Pin{Version: "v0.1.0", Ready: true}); e != nil {
		t.Fatal(e)
	}
	if got := m.releaseSelectionText(); got != "cxz release pinned: v0.1.0" {
		t.Fatal(got)
	}
}
