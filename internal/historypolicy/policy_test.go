package historypolicy

import "testing"

func TestPolicyDefaultsAndValidation(t *testing.T) {
	p, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, b, n := p.Limits()
	if d != 100*MiB || b != 10*MiB || n != 200 {
		t.Fatal(d, b, n)
	}
	p.Disabled = true
	if d, _, _ := p.Limits(); d != 0 {
		t.Fatal(d)
	}
	for _, v := range []Policy{{MaxMiB: -1}, {MaxMiB: 10241}, {WindowMiB: 1025}, {WindowTurns: -1}} {
		if v.Validate() == nil {
			t.Fatal("invalid policy", v)
		}
	}
	root := t.TempDir()
	p.MaxMiB = 250
	if err = Save(root, p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil || got != p {
		t.Fatal(got, err)
	}
}

// The vendor stream is bounded on its own, and turning retention off turns it
// off with everything else rather than leaving one budget running.
func TestVendorStreamBudget(t *testing.T) {
	if n := (Policy{}).RawLimit(); n != 20*MiB {
		t.Fatal("default", n)
	}
	if n := (Policy{RawMiB: 5}).RawLimit(); n != 5*MiB {
		t.Fatal("configured", n)
	}
	if n := (Policy{Disabled: true, RawMiB: 5}).RawLimit(); n != 0 {
		t.Fatal("a disabled policy kept a budget", n)
	}
	// The two budgets are independent: a session can keep little stream and
	// much conversation, which is the point of having both.
	p := Policy{MaxMiB: 500, RawMiB: 5}
	disk, _, _ := p.Limits()
	if disk != 500*MiB || p.RawLimit() != 5*MiB {
		t.Fatal(disk, p.RawLimit())
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if (Policy{RawMiB: -1}).Validate() == nil || (Policy{RawMiB: 20481}).Validate() == nil {
		t.Fatal("an impossible vendor budget was accepted")
	}
}
