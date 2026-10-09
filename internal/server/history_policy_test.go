package server

import (
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypolicy"
)

// A runtime keeps the budgets it was given. Zero still means "use the default",
// which is the policy's rule and not the wire's -- so a round trip through the
// API must not turn an unset budget into a configured zero.
func TestHistoryPolicyRoundTripKeepsUnsetBudgets(t *testing.T) {
	s := &Server{root: t.TempDir()}
	ctx := t.Context()
	out, err := s.GetHistoryPolicy(ctx, &api.Empty{})
	if err != nil || out.MaxMib != 0 || out.RawMib != 0 {
		t.Fatal("a fresh runtime reported configured budgets", out, err)
	}
	if out, err = s.SetHistoryPolicy(ctx, &api.HistoryPolicy{MaxMib: 25, RawMib: 5}); err != nil {
		t.Fatal(err)
	}
	if out.MaxMib != 25 || out.RawMib != 5 || out.Disabled {
		t.Fatal("not what was set", out)
	}
	// And it is on disk as the policy, with the defaults the policy applies.
	p, err := historypolicy.Load(s.root)
	if err != nil || p.MaxMiB != 25 || p.RawMiB != 5 {
		t.Fatal(p, err)
	}
	if disk, _, _ := p.Limits(); disk != 25*historypolicy.MiB || p.RawLimit() != 5*historypolicy.MiB {
		t.Fatal("limits", disk, p.RawLimit())
	}
	// Turning retention off survives the round trip, because it is the one
	// setting that means "remove nothing" rather than "use the default".
	if out, err = s.SetHistoryPolicy(ctx, &api.HistoryPolicy{Disabled: true}); err != nil || !out.Disabled {
		t.Fatal(out, err)
	}
	if p, err = historypolicy.Load(s.root); err != nil || !p.Disabled {
		t.Fatal(p, err)
	}
	if disk, _, _ := p.Limits(); disk != 0 || p.RawLimit() != 0 {
		t.Fatal("a disabled policy kept a budget", disk, p.RawLimit())
	}
}
