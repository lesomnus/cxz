package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path/filepath"
	"testing"
)

type accountStatusFixture struct {
	*fixture
	loggedOut      bool
	called         int
	agent, backend string
}

func (f *accountStatusFixture) AccountStatus(_ context.Context, alias, agent, backend string) (accounts.AuthStatus, error) {
	f.called++
	f.agent, f.backend = agent, backend
	if backend == accounts.ProjectLocalOAuth {
		return accounts.AuthStatus{State: "session_scoped", Scope: "session"}, nil
	}
	return accounts.AuthStatus{State: map[bool]string{false: "ready", true: "login_required"}[f.loggedOut], Scope: "account", CredentialPresent: !f.loggedOut, CanLogin: f.loggedOut, CanLogout: !f.loggedOut}, nil
}
func (f *accountStatusFixture) LogoutAccount(_ context.Context, alias, agent, backend string) error {
	if backend != accounts.BrokeredAccessToken {
		return status.Error(codes.FailedPrecondition, "session-scoped login")
	}
	f.loggedOut = true
	return nil
}
func TestAccountStatusAndLogoutResolveRegisteredAccount(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &accountStatusFixture{fixture: &fixture{p: &api.Project{Id: "p"}, s: &api.Session{Id: "s"}}}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "codex"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	got, err := stack.Account().Status(ctx, accountRef("work"))
	if err != nil || got.GetState() != resource.AccountAuthStatus_STATE_READY || got.GetCanLogin() || !got.GetCanLogout() || got.GetVendorVerified() || f.agent != "codex" || f.backend != accounts.BrokeredAccessToken {
		t.Fatal(got, err)
	}
	before := f.called
	_, err = stack.Account().Status(ctx, accountRef("missing"))
	if status.Code(err) != codes.NotFound || f.called != before {
		t.Fatal("unknown account reached credential runtime", err)
	}
	got, err = stack.Account().Logout(ctx, accountRef("work"))
	if err != nil || got.GetState() != resource.AccountAuthStatus_STATE_LOGIN_REQUIRED || !got.GetCanLogin() || got.GetCredentialPresent() {
		t.Fatal(got, err)
	}
	_, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "personal", Agent: "claude"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	got, err = stack.Account().Status(ctx, accountRef("personal"))
	if err != nil || got.GetState() != resource.AccountAuthStatus_STATE_SESSION_SCOPED || got.GetCanLogin() || got.GetCanLogout() {
		t.Fatal(got, err)
	}
	_, err = stack.Account().Logout(ctx, accountRef("personal"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}
