package filemap

import (
	"path/filepath"
	"testing"
)

func TestShareVariableUsesSelectedStateAndPreservesMapping(t *testing.T) {
	t.Setenv("CXZ_SHARE_DIR", filepath.Join(t.TempDir(), "not-the-cxz-share"))
	mappings := []Mapping{{Src: "${CXZ_SHARE_DIR}/CLAUDE.md", Dst: "${AGENT_CONFIG_DIR}/CLAUDE.md", Agent: "claude"}, {Src: "${CXZ_SHARE_DIR}/foo", Dst: "${WORKSPACE}/shared"}}
	for _, content := range []string{"first installation", "second installation"} {
		root := filepath.Join(t.TempDir(), "state with spaces 한글")
		put(t, filepath.Join(root, "share", "CLAUDE.md"), content)
		put(t, filepath.Join(root, "share", "foo", "bar", "baz.txt"), "nested")
		bundle, err := Snapshot(root, mappings)
		if err != nil || len(bundle.Files) != 2 || string(bundle.Files[0].Content) != content || bundle.Files[0].Dst != "${AGENT_CONFIG_DIR}/CLAUDE.md" || bundle.Files[1].Dst != "${WORKSPACE}/shared/bar/baz.txt" {
			t.Fatal(bundle, err)
		}
		if mappings[0].Src != "${CXZ_SHARE_DIR}/CLAUDE.md" {
			t.Fatal("snapshot expanded the stored preference")
		}
	}
}
