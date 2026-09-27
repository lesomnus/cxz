package cxzupdate

import (
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/versionpin"
	"path/filepath"
	"testing"
)

func TestReleasePinOverridesAutomaticPolicy(t *testing.T) {
	root := t.TempDir()
	if e := versionpin.Save(root, versionpin.Pin{Version: "v0.1.0", Ready: true}); e != nil {
		t.Fatal(e)
	}
	if e := SetPolicy(root, true); e == nil {
		t.Fatal("enabled automatic update over pin")
	}
	// Even a stale client writing enabled cannot bypass the release pin.
	if e := core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), map[string]bool{"enabled": true}); e != nil {
		t.Fatal(e)
	}
	p, e := Policy(root)
	if e != nil || p.Active() {
		t.Fatal("pin bypassed", e)
	}
	if e = versionpin.Clear(root); e != nil {
		t.Fatal(e)
	}
	p, e = Policy(root)
	if e != nil || !p.Active() {
		t.Fatal("unpin did not restore default policy", e)
	}
}
