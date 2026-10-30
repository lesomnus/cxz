package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPressureDoesNotRunDatabaseRetention(t *testing.T) {
	root := t.TempDir()
	e := Engine{Roots: []string{root}, Collect: func(context.Context, Policy, time.Time, bool) ([]Candidate, error) { return nil, nil }, Probe: func(string) (Space, error) { return Space{Device: "disk", Total: 100, Available: 1}, nil }}
	r := NewRunner(root, e)
	calls := 0
	r.Retain = func(context.Context, Policy, time.Time) error { calls++; return nil }
	r.once(t.Context(), Default(), time.Now())
	if calls != 0 {
		t.Fatal("database writes used as emergency reclaim")
	}
	p := Default()
	p.PressureLowPercent = 0
	r.once(t.Context(), p, time.Now())
	if calls != 1 {
		t.Fatal("ordinary retention never ran")
	}
	if _, err := os.Stat(filepath.Join(root, "maintenance-status.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDisabledRunnerStillPropagatesPolicyAndCancels(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "maintenance.json"), []byte(`{"disabled":true}`), 0600)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	configured := false
	r := NewRunner(root, Engine{})
	r.Configure = func(_ context.Context, p Policy) error { configured = p.Disabled; cancel(); return nil }
	r.Run(ctx)
	if !configured {
		t.Fatal("disabled policy was not propagated")
	}
}
