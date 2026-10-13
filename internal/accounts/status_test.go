package accounts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCentralStatusLogoutPreservesIdentityAndOtherAccounts(t *testing.T) {
	root := t.TempDir()
	for _, account := range []string{"work", "other"} {
		// Existing broker fixtures publish valid, pinned subscription credentials.
		seedCentral(t, root, account, "subject-"+account, "synthetic-token")
		if _, err := IssueGrant(root, "project", account); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := IssueAuxiliaryGrant(root, "work"); err != nil {
		t.Fatal(err)
	}
	state, err := CentralStatus(root, "work")
	if err != nil || state.State != "ready" || !state.CredentialPresent || state.CanLogin || !state.CanLogout {
		t.Fatal(state, err)
	}
	pin, _ := os.ReadFile(filepath.Join(Dir(centralRoot(root), "work"), "subject.json"))
	if err := CentralLogout(context.Background(), root, "work"); err != nil {
		t.Fatal(err)
	}
	state, err = CentralStatus(root, "work")
	if err != nil || state.State != "login_required" || state.CredentialPresent || !state.CanLogin || state.CanLogout {
		t.Fatal(state, err)
	}
	if _, err := IssueGrant(root, "project", "work"); err == nil {
		t.Fatal("logged-out account supplied tokens")
	}
	state, err = CentralStatus(root, "other")
	if err != nil || state.State != "ready" {
		t.Fatal("other account was logged out", state, err)
	}
	remaining, _ := filepath.Glob(filepath.Join(root, "central", "bindings", "*.json"))
	if len(remaining) != 1 {
		t.Fatal("account grants remained or other grants were deleted", remaining)
	}
	got, _ := os.ReadFile(filepath.Join(Dir(centralRoot(root), "work"), "subject.json"))
	if string(got) != string(pin) {
		t.Fatal("logout removed immutable identity pin")
	}
	if err := CentralLogout(context.Background(), root, "work"); err != nil {
		t.Fatal("logout must be idempotent", err)
	}
}

func TestCentralStatusMissingDoesNotCreateAnAccountAndCancelledLogoutPreservesLogin(t *testing.T) {
	root := t.TempDir()
	state, err := CentralStatus(root, "work")
	if err != nil || state.State != "login_required" || !state.CanLogin {
		t.Fatal(state, err)
	}
	if _, err := os.Stat(filepath.Join(root, "central")); !os.IsNotExist(err) {
		t.Fatal("read-only status created account directories", err)
	}
	seedCentral(t, root, "work", "subject-work", "synthetic-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CentralLogout(ctx, root, "work"); err == nil {
		t.Fatal("cancelled logout succeeded")
	}
	state, err = CentralStatus(root, "work")
	if err != nil || state.State != "ready" {
		t.Fatal("cancelled logout changed credentials", state, err)
	}
}
