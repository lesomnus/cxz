package auxiliary

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

func TestHelperCatalogReusesDedicatedAccountCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "provider")
	// Only initialization is permitted: a model turn or login would fail.
	script := `#!/bin/sh
read -r request
case "$request" in
 *'"subtype":"initialize"'*) ;;
 *) exit 9;;
esac
printf '%s\n' '{"type":"control_response","response":{"request_id":"init","subtype":"success","response":{"models":[{"value":"fixture"}]}}}'
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	probe := func(alias, task string) (Output, error) {
		raw, _ := json.Marshal(HelperInput{Input: Input{Task: task, Profile: Profile{Account: alias, Agent: "claude", Backend: accounts.ProjectLocalOAuth}}, Binary: bin})
		var buf bytes.Buffer
		err := Serve(t.Context(), root, bytes.NewReader(raw), &buf)
		var out Output
		if err == nil {
			err = json.Unmarshal(buf.Bytes(), &out)
		}
		return out, err
	}
	if out, err := probe("work", "models"); err != nil || !out.NeedsLogin {
		t.Fatalf("missing auth: %+v %v", out, err)
	}
	if _, err := probe("work", "summary"); err == nil {
		t.Fatal("generation accepted missing auth")
	}
	if err := accounts.Install(root, "work", "claude", []byte(`{"claudeAiOauth":{"accessToken":"fixture-only"}}`)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if out, err := probe("work", "models"); err != nil || out.NeedsLogin || len(out.Models) != 1 {
			t.Fatalf("stored auth not reused: %+v %v", out, err)
		}
	}
	if out, err := probe("other", "models"); err != nil || !out.NeedsLogin {
		t.Fatalf("cross-account credential reuse: %+v %v", out, err)
	}
	lock, err := core.Lock(filepath.Join(accounts.Dir(root, "work"), "login.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if out, err := probe("work", "models"); err == nil || out.NeedsLogin {
		t.Fatalf("busy profile treated as missing login: %+v %v", out, err)
	}
}
