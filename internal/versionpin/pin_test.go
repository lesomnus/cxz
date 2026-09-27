package versionpin

import (
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/metadata"
	"os"
	"path/filepath"
	"testing"
)

func TestPinSurvivesSwitchAndRestoresPolicy(t *testing.T) {
	root := t.TempDir()
	off := false
	core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), map[string]bool{"enabled": off})
	p := Pin{Version: "v0.1.0", Generation: core.ID()}
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	if !Pending(root) || Check(root) == nil {
		t.Fatal("pin not enforced")
	}
	p.Version = "v0.2.0"
	p.Ready = true
	if e := Save(root, p); e != nil {
		t.Fatal(e)
	}
	got, e := Load(root)
	if e != nil || got.Version != p.Version || got.PreviousEnabled == nil || *got.PreviousEnabled {
		t.Fatal(got, e)
	}
	if e = Clear(root); e != nil {
		t.Fatal(e)
	}
	if Pending(root) || Check(root) != nil {
		t.Fatal("unpin did not clear")
	}
	data, _ := os.ReadFile(filepath.Join(root, "cxz-update-policy.json"))
	if string(data) != "{\n  \"enabled\": false\n}" {
		t.Fatal(string(data))
	}
}
func TestUseFenceOnlyAdmitsControllerResume(t *testing.T) {
	root := t.TempDir()
	id := core.ID()
	core.WriteJSON(filepath.Join(root, "use-barrier.json"), Barrier{Generation: id, Phase: "resuming"})
	ctx := context.Background()
	if allowed(ctx, root, "/Session/Send") || allowed(ctx, root, "/Session/Resume") {
		t.Fatal("user mutation admitted")
	}
	if !allowed(ctx, root, "/Session/List") {
		t.Fatal("read rejected")
	}
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("cxz-use-id", id))
	if !allowed(ctx, root, "/Session/Resume") || allowed(ctx, root, "/Session/Send") {
		t.Fatal("controller bypass too broad")
	}
}
func TestValidatePinnedRelease(t *testing.T) {
	for _, v := range []string{"main", "edge", "../v0.1.0", "v0.1.0;sh", ""} {
		if Validate(v) == nil {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"v0.1.0", "v1.2.3-rc.1"} {
		if e := Validate(v); e != nil {
			t.Fatal(e)
		}
	}
}
