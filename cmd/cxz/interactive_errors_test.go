package main

import (
	"context"
	"errors"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/configtrust"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/protobuf/proto"
)

func TestTrustRetryOnlyAfterConsent(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interactive, yes bool
		err              error
		calls, prompts   int
	}{
		{"accept", true, true, &configtrust.Required{}, 2, 1},
		{"exit", true, false, &configtrust.Required{}, 1, 1},
		{"failfast", false, true, &configtrust.Required{}, 1, 0},
		{"other error", true, true, errors.New("unavailable"), 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &api.ProjectRequest{Workspace: "/project", ClientId: "stable", Recreate: true, Confirmed: true, Account: "work"}
			expected := proto.Clone(r).(*api.ProjectRequest)
			calls, prompts := 0, 0
			_, err := withConfigTrust(t.Context(), r, tc.interactive, func(_ context.Context, in *api.ProjectRequest) (*api.Session, error) {
				calls++
				if calls == 1 {
					if in.TrustConfig {
						t.Fatal("implicit trust")
					}
					return nil, tc.err
				}
				expected.TrustConfig = true
				if !proto.Equal(in, expected) {
					t.Fatal("retry changed request", in)
				}
				return &api.Session{}, nil
			}, func(context.Context, error) (bool, error) { prompts++; return tc.yes, nil })
			if calls != tc.calls || prompts != tc.prompts {
				t.Fatal(calls, prompts)
			}
			if tc.calls == 2 && err != nil {
				t.Fatal(err)
			}
			if tc.calls == 1 && r.TrustConfig {
				t.Fatal("saved unapproved trust")
			}
		})
	}
}
func TestExitOnErrorFlagBeforeAndAfterCommand(t *testing.T) {
	for _, args := range [][]string{{"-x", "up"}, {"up", "-x"}, {"--exit-on-error", "up"}, {"up", "--exit-on-error"}} {
		root := newRoot(t.TempDir())
		var reached bool
		for _, c := range root.Commands {
			if c.Name == "up" {
				c.Handler = onRun(func(_ context.Context, c *xli.Command) error { reached = exitOnError(c); return nil })
			}
		}
		result := xlitest.Run(t, root, args...)
		if result.Err != nil || !reached {
			t.Fatal(args, result.Err, reached)
		}
	}
}
