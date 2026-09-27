package versionpin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChannelPinUnpinAndInterruptedSwitch(t *testing.T) {
	root := t.TempDir()
	p := Pin{Version: "v0.1.2", Channel: "stable", Generation: "first"}
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	if !Pending(root) || Check(root) == nil {
		t.Fatal("unfinished switch allowed")
	}
	p.Ready = true
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	if Pending(root) || Check(root) != nil {
		t.Fatal("completed channel treated as pin")
	}
	if c, e := Channel(root); e != nil || c != "stable" {
		t.Fatal(c, e)
	}
	enabled := func() bool {
		var p struct{ Enabled bool }
		b, e := os.ReadFile(filepath.Join(root, "cxz-update-policy.json"))
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, &p); e != nil {
			t.Fatal(e)
		}
		return p.Enabled
	}
	if !enabled() {
		t.Fatal("channel not enabled")
	}
	p = Pin{Version: "v0.1.0", Generation: "second", Ready: true}
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	if !p.Pinned() || Check(root) == nil || enabled() {
		t.Fatal("explicit version not pinned")
	}
	if e := Clear(root); e != nil {
		t.Fatal(e)
	}
	if c, e := Channel(root); e != nil || c != "stable" || !enabled() {
		t.Fatal("previous channel/policy not restored", c, e)
	}
	p = Pin{Version: "source-aaaaaaaaaaaa", Channel: "edge", Generation: "third", Ready: true}
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	if c, e := Channel(root); e != nil || c != "edge" || Check(root) != nil {
		t.Fatal(c, e)
	}
}
