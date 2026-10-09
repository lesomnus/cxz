package auxkind

import (
	"testing"

	"github.com/lesomnus/cxz/resource"
)

// The two surfaces name the same kind differently, so the translation has to be
// exact in both directions -- and has to have no answer for a kind it does not
// know, which is what lets the layer above refuse instead of guessing.
func TestEveryKindSurvivesTheRoundTrip(t *testing.T) {
	for _, name := range []string{Summary, Suggestion, Title} {
		k := Of(name)
		if k == resource.AuxKind_AUX_KIND_UNSPECIFIED || Name(k) != name {
			t.Fatalf("%q became %v", name, k)
		}
	}
	if Name(resource.AuxKind_AUX_KIND_UNSPECIFIED) != "" {
		t.Fatal("the unset kind was given a name")
	}
	if Of("haiku") != resource.AuxKind_AUX_KIND_UNSPECIFIED {
		t.Fatal("an unknown name was given a kind")
	}
	// A kind from a newer client drops out rather than arriving as a summary.
	if got := Names([]resource.AuxKind{resource.AuxKind_AUX_KIND_SUMMARY, resource.AuxKind(99)}); len(got) != 1 || got[0] != Summary {
		t.Fatal(got)
	}
	if got := Enums([]string{"haiku", Title}); len(got) != 1 || got[0] != resource.AuxKind_AUX_KIND_TITLE {
		t.Fatal(got)
	}
}
