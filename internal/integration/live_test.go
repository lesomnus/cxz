package integration

import (
	"os"
	"os/exec"
	"testing"
)

// Opt-in paid acceptance uses an already authenticated owned project. It never
// copies host refresh tokens or manufactures a shared subscription login.
func TestLiveClaude(t *testing.T) {
	if os.Getenv("CXZ_LIVE_TEST") != "1" {
		t.Skip("set CXZ_LIVE_TEST=1 with CXZ_LIVE_STATE, CXZ_LIVE_PROJECT and CXZ_LIVE_ACCOUNT")
	}
	state, project, account := os.Getenv("CXZ_LIVE_STATE"), os.Getenv("CXZ_LIVE_PROJECT"), os.Getenv("CXZ_LIVE_ACCOUNT")
	if state == "" || project == "" || account == "" {
		t.Fatal("explicit disposable installed state, project and account required")
	}
	cmd := exec.Command("node", "scripts/probes/owned-session-live.mjs", state, project, "claude", account)
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("live acceptance failed: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
