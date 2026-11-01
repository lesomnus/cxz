package server

import (
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestAccountOperationsRequireManager(t *testing.T) {
	s := &Server{}
	for name, err := range map[string]error{
		"status":          must(s.AccountStatus(t.Context(), "work", "codex", accounts.BrokeredAccessToken)),
		"logout":          s.LogoutAccount(t.Context(), "work", "codex", accounts.BrokeredAccessToken),
		"erase preflight": s.AccountEraseAllowed(t.Context()),
		"erase":           s.EraseAccount(t.Context(), "work", "codex", accounts.BrokeredAccessToken),
	} {
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
func TestAccountAuthenticationDelegatesToManager(t *testing.T) {
	s := &Server{manager: &workspace.Manager{Root: t.TempDir(), Owner: "owner"}}
	got, err := s.AccountStatus(t.Context(), "work", "codex", accounts.BrokeredAccessToken)
	if err != nil || got.State != "login_required" || !got.CanLogin {
		t.Fatal(got, err)
	}
	if err := s.LogoutAccount(t.Context(), "work", "codex", accounts.BrokeredAccessToken); err != nil {
		t.Fatal(err)
	}
}
