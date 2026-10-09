package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type purgeDaemon struct {
	daemon
	session string
	calls   int
}

func (d *purgeDaemon) PurgeSession(_ context.Context, in *api.SessionPurgeInput, _ ...grpc.CallOption) (*api.SessionPurgeReply, error) {
	d.calls++
	d.session = in.SessionId
	return &api.SessionPurgeReply{SessionId: in.SessionId, DryRun: in.DryRun}, nil
}
func (d *purgeDaemon) GetHistoryPolicy(context.Context, *api.Empty, ...grpc.CallOption) (*api.HistoryPolicy, error) {
	d.calls++
	return &api.HistoryPolicy{MaxMib: 25}, nil
}

// The session names the connection that holds it, and the reply comes back
// under the name the caller used: it addressed a prefixed session and reads the
// answer against that, not against the id the far side knows.
func TestPurgeRoutesAndAnswersUnderTheCallersName(t *testing.T) {
	a, b := &purgeDaemon{}, &purgeDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	out, err := c.PurgeSession(t.Context(), &api.SessionPurgeInput{SessionId: "b::S", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.calls != 0 || b.session != "S" {
		t.Fatal("wrong manager or session", a.calls, b.session)
	}
	if out.SessionId != "b::S" || !out.DryRun {
		t.Fatal("the reply renamed the session", out)
	}
	// The budgets belong to an installation, so they follow the connection the
	// caller is looking at rather than a session of their own.
	if _, err = c.GetHistoryPolicy(c.ContextFor(t.Context(), "b::S"), &api.Empty{}); err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatal("the policy ignored the connection", b.calls)
	}
}
