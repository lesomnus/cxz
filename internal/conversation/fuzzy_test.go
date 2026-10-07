package conversation

import "testing"

// Fuzzy matching is per line, and that is the property that makes it useful: a
// query's letters can be found scattered across any paragraph.
func TestFuzzyMatchesALineNotAParagraph(t *testing.T) {
	paragraph := "the certificate was signed by the installation root\nand cxz web could not start\n"
	score, lo, hi, ok := fuzzyMatch(paragraph, "cxzweb", true)
	if !ok {
		t.Fatal("no match")
	}
	if paragraph[lo:hi] != "cxz web" {
		t.Fatalf("matched %q", paragraph[lo:hi])
	}
	if score <= 0 {
		t.Fatal(score)
	}
	// The same letters, spread across a sentence, are not a match: c, x, z
	// appear in order in the first line and it is still not what was meant.
	if _, _, _, ok = fuzzyMatch("the certificate was signed by the installation root\n", "cxzweb", true); ok {
		t.Fatal("a scavenger hunt counted as a fuzzy match")
	}
}

// A tighter, word-starting run beats a loose one, which is what ranks the line
// a person meant above the line that merely contains the letters.
func TestFuzzyPrefersTightWordStartingRuns(t *testing.T) {
	tight, _, _, ok := fuzzyMatch("cxz web up", "cxzweb", true)
	if !ok {
		t.Fatal("exact run did not match")
	}
	loose, _, _, ok := fuzzyMatch("c x z w e b", "cxzweb", true)
	if !ok {
		t.Fatal("spaced run did not match")
	}
	if tight <= loose {
		t.Fatalf("tight %d is not better than loose %d", tight, loose)
	}
	if _, _, _, ok = fuzzyMatch("CXZ WEB", "cxzweb", false); ok {
		t.Fatal("case was ignored when it was not asked for")
	}
	if _, _, _, ok = fuzzyMatch("CXZ WEB", "cxzweb", true); !ok {
		t.Fatal("case was not ignored when it was asked for")
	}
	if _, _, _, ok = fuzzyMatch("anything", "", true); ok {
		t.Fatal("an empty query matched")
	}
}

// Tightening is what makes the span the word rather than everything from the
// first letter onwards.
func TestFuzzyTightensTheSpan(t *testing.T) {
	text := "relay relay relay web"
	_, lo, hi, ok := fuzzyMatch(text, "relayweb", true)
	if !ok {
		t.Fatal("no match")
	}
	if text[lo:hi] != "relay web" {
		t.Fatalf("matched %q instead of the last run", text[lo:hi])
	}
}
