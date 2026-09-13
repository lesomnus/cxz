package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSessionScopedInternalStatus(t *testing.T) {
	root := t.TempDir()
	if err := accounts.Install(accounts.SessionRoot(root, "creation"), "work", "claude", []byte(`{"claudeAiOauth":{"accessToken":"synthetic"}}`)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"creation", "different"} {
		got := xlitest.Run(t, newRoot(root), "_account-status", "--session-key", key, "work", "claude", accounts.ProjectLocalOAuth)
		if (got.Err == nil) != (key == "creation") {
			t.Fatalf("%s: %+v", key, got)
		}
	}
}

func projectLoginRequired(t *testing.T, alias string) error {
	t.Helper()
	s, err := status.New(codes.FailedPrecondition, "missing login").WithDetails(&errdetails.ErrorInfo{Domain: "cxz.auth", Reason: "PROJECT_LOGIN_REQUIRED", Metadata: map[string]string{"account": alias}})
	if err != nil {
		t.Fatal(err)
	}
	return s.Err()
}

func TestOpenWithProjectLogin(t *testing.T) {
	for _, tc := range []struct {
		name              string
		interactive       bool
		account           string
		initial, loginErr error
		opens, logins     int
	}{
		{"authenticated", true, "main", nil, nil, 1, 0},
		{"first-login", true, "main", projectLoginRequired(t, "main"), nil, 2, 1},
		{"resume-account", true, "", projectLoginRequired(t, "main"), nil, 2, 1},
		{"script", false, "main", projectLoginRequired(t, "main"), nil, 1, 0},
		{"cancel", true, "main", projectLoginRequired(t, "main"), errors.New("canceled"), 1, 1},
		{"wrong-account", true, "other", projectLoginRequired(t, "main"), nil, 1, 0},
		{"corrupt-or-central", true, "main", status.Error(codes.FailedPrecondition, "needs login"), nil, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &api.ProjectRequest{Workspace: "/work/project", Account: tc.account, Recreate: true, Confirmed: true, TrustConfig: true, NewSession: true, ClientId: "request", Model: "model"}
			opens, logins := 0, 0
			_, err := openWithProjectLogin(context.Background(), r, tc.interactive, func(_ context.Context, retry *api.ProjectRequest) (*api.Session, error) {
				opens++
				if opens == 1 {
					return &api.Session{}, tc.initial
				}
				if retry.Recreate || retry.Account != "main" || retry.ClientId != r.ClientId || !retry.NewSession || retry.Model != r.Model || retry.Workspace != r.Workspace {
					t.Fatal("incorrect retry", retry)
				}
				return &api.Session{Id: "session"}, nil
			}, func(_ context.Context, alias, key string) error {
				logins++
				if alias != "main" || key != r.ClientId {
					t.Fatal(alias)
				}
				return tc.loginErr
			})
			if opens != tc.opens || logins != tc.logins {
				t.Fatalf("open=%d login=%d err=%v", opens, logins, err)
			}
			if !r.Recreate || r.Account != tc.account {
				t.Fatal("mutated original request")
			}
			if tc.opens == 2 && err != nil {
				t.Fatal(err)
			}
			if tc.name == "script" && (err == nil || !strings.Contains(err.Error(), `--session <session>`)) {
				t.Fatal(err)
			}
			if tc.loginErr != nil && !errors.Is(err, tc.loginErr) {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectLoginRetryIsBounded(t *testing.T) {
	errLogin := projectLoginRequired(t, "main")
	opens, logins := 0, 0
	_, err := openWithProjectLogin(context.Background(), &api.ProjectRequest{Account: "main"}, true, func(context.Context, *api.ProjectRequest) (*api.Session, error) { opens++; return nil, errLogin }, func(context.Context, string, string) error { logins++; return nil })
	if err == nil || opens != 2 || logins != 1 {
		t.Fatalf("%d %d %v", opens, logins, err)
	}
}

func TestResumeLoginUsesOriginalSessionKey(t *testing.T) {
	st, err := status.New(codes.FailedPrecondition, "login").WithDetails(&errdetails.ErrorInfo{Domain: "cxz.auth", Reason: "PROJECT_LOGIN_REQUIRED", Metadata: map[string]string{"account": "main", "session_key": "original-creation"}})
	if err != nil {
		t.Fatal(err)
	}
	opens := 0
	_, err = openWithProjectLogin(context.Background(), &api.ProjectRequest{Account: "main", ClientId: "resume-operation"}, true, func(context.Context, *api.ProjectRequest) (*api.Session, error) {
		opens++
		if opens == 1 {
			return nil, st.Err()
		}
		return &api.Session{}, nil
	}, func(_ context.Context, alias, key string) error {
		if alias != "main" || key != "original-creation" {
			t.Fatalf("wrong login target: %s %s", alias, key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
