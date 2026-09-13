package distribution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestStableReleaseValidation(t *testing.T) {
	for _, v := range []string{"../1.0.0", "v1.2.3", "1.2.3-beta", "1.2", "1.2.3/gh"} {
		if ValidVersion(v) {
			t.Fatal(v)
		}
	}
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "releases.json"), map[string]string{"claude": "3.1.2", "codex": "../bad"}); err != nil {
		t.Fatal(err)
	}
	if SelectedVersion(root, "claude", "1.0.0") != "3.1.2" || SelectedVersion(root, "codex", "1.0.0") != "1.0.0" {
		t.Fatal("unsafe release selector")
	}
}
func TestLatestStableLive(t *testing.T) {
	if os.Getenv("CXZ_RELEASE_CHECK_TEST") != "1" {
		t.Skip("opt-in official release lookup")
	}
	for _, kind := range []string{"claude", "codex", "gh"} {
		v, err := Latest(context.Background(), kind)
		if err != nil {
			t.Fatal(kind, err)
		}
		t.Log(kind, v)
	}
}
