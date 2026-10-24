package projectid

import (
	"crypto/sha256"
	"testing"

	"github.com/google/uuid"
)

func TestLegacyMappingPreservesExistingResourceUUID(t *testing.T) {
	legacy := "0123456789abcdef01234567"
	h := sha256.Sum256([]byte("cxz.resource:" + legacy))
	want := uuid.UUID(h[:16])
	want[6] = want[6]&15 | 0x80
	want[8] = want[8]&63 | 0x80
	want[9] = 7
	if got := Canonical(legacy); got != want.String() {
		t.Fatalf("%s != %s", got, want)
	}
	if Canonical(want.String()) != want.String() || String(Bytes(legacy)) != want.String() {
		t.Fatal("UUID was reallocated")
	}
	if !Same(legacy, want.String()) || Same(legacy, New()) || Same("", "") {
		t.Fatal("identity comparison crossed project scope")
	}
}

func TestNewProjectIdentityIsIndependent(t *testing.T) {
	a, b := New(), New()
	if a == b || Canonical(a) != a {
		t.Fatal(a, b)
	}
	v := uuid.MustParse(a)
	if v[9] != 7 || v.Version() != 8 {
		t.Fatal("not a payday project UUID", a)
	}
}
