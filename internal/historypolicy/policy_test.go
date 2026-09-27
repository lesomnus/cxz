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
