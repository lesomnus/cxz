package workspace

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/dockerx"
	"testing"
)

func TestAssetEnginePathUsesDaemonMounts(t *testing.T) {
	var manager dockerx.Container
	if err := json.Unmarshal([]byte(`{"Mounts":[{"Source":"/engine/state","Destination":"/var/lib/cxz","Type":"volume"},{"Source":"/engine/asset-disk","Destination":"/var/lib/cxz/assets","Type":"bind"}]}`), &manager); err != nil {
		t.Fatal(err)
	}
	got, err := assetEnginePath("/var/lib/cxz/assets/exports/project", manager)
	if err != nil || got != "/engine/asset-disk/exports/project" {
		t.Fatal("manager path leaked into engine mount", got, err)
	}
	if _, err := assetEnginePath("/var/lib/cxz-other/assets", manager); err == nil {
		t.Fatal("accepted path outside mount")
	}
}
