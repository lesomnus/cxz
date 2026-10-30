package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPressurePriorityRecoveryAndFilesystemIsolation(t *testing.T) {
	p := Default()
	var order []Kind
	spaces := map[string]Space{"a": {Device: "a", Total: 100, Available: 4}, "b": {Device: "b", Total: 100, Available: 50}}
	e := Engine{Roots: []string{"a", "b"}, Probe: func(path string) (Space, error) { return spaces[path[:1]], nil }}
	e.Collect = func(_ context.Context, _ Policy, _ time.Time, pressure bool) ([]Candidate, error) {
		if !pressure {
			return nil, nil
		}
		var cs []Candidate
		for _, k := range []Kind{Cache, Binaries, Logs, Temporary, Residue} {
			cs = append(cs, Candidate{Path: "a/file", Kind: k, Remove: func(context.Context) (bool, error) {
				order = append(order, k)
				s := spaces["a"]
				s.Available += 2
				spaces["a"] = s
				return true, nil
			}})
		}
		cs = append(cs, Candidate{Path: "b/file", Kind: Residue, Remove: func(context.Context) (bool, error) {
			t.Fatal("healthy filesystem was cleaned under pressure elsewhere")
			return false, nil
		}})
		return cs, nil
	}
	r := e.Pass(t.Context(), p, time.Now())
	if !reflect.DeepEqual(order, []Kind{Residue, Temporary, Logs}) || r.Removed != 3 || r.Volumes[0].Pressure {
		t.Fatalf("order=%v report=%+v", order, r)
	}
	// Once pressure begins it persists inside the hysteresis band.
	e.pressured["a"] = true
	spaces["a"] = Space{Device: "a", Total: 100, Available: 7}
	e.Collect = func(context.Context, Policy, time.Time, bool) ([]Candidate, error) { return nil, nil }
	r = e.Pass(t.Context(), p, time.Now())
	if !r.Volumes[0].Pressure {
		t.Fatal("pressure forgotten below recovery watermark")
	}
	spaces["a"] = Space{Device: "a", Total: 100, Available: 10}
	r = e.Pass(t.Context(), p, time.Now())
	if r.Volumes[0].Pressure {
		t.Fatal("pressure persisted after recovery")
	}
}

func TestPassBudgetAndCancellation(t *testing.T) {
	removed := 0
	e := Engine{Collect: func(context.Context, Policy, time.Time, bool) ([]Candidate, error) {
		var cs []Candidate
		for range 1000 {
			cs = append(cs, Candidate{Remove: func(context.Context) (bool, error) { removed++; return true, nil }})
		}
		return cs, nil
	}}
	r := e.Pass(t.Context(), Default(), time.Now())
	if removed != 128 || r.Examined != 128 {
		t.Fatalf("unbounded pass: %+v", r)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	e.Pass(ctx, Default(), time.Now())
	if removed != 128 {
		t.Fatal("deleted after cancellation")
	}
}

func TestPolicyDefaultsExplicitDisableAndValidation(t *testing.T) {
	root := t.TempDir()
	p, err := Load(root)
	if err != nil || p.AuditDays != 90 {
		t.Fatal(p, err)
	}
	path := filepath.Join(root, "maintenance.json")
	if err = os.WriteFile(path, []byte(`{"audit_days":0,"trash_days":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err = Load(root)
	if err != nil || p.AuditDays != 0 || p.TrashDays != 7 || p.RecentVersions != 2 {
		t.Fatal(p, err)
	}
	for _, body := range []string{`{"interval_minutes":0}`, `{"pressure_low_percent":10,"pressure_high_percent":5}`, `{"audit_mib":128}`, `{"audit_days":-1}`, `null`, `{} {}`} {
		os.WriteFile(path, []byte(body), 0600)
		if _, err = Load(root); err == nil {
			t.Fatalf("accepted invalid policy %s", body)
		}
	}
}
