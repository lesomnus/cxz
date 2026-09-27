package sessionalias

import (
	"strings"
	"testing"

	"github.com/lesomnus/payday/slug"
)

func TestWords(t *testing.T) {
	seen := map[string]bool{}
	for _, word := range Words {
		if !Valid(word) || seen[word] {
			t.Fatal("invalid or duplicate word", word)
		}
		seen[word] = true
	}
	for _, good := range []string{"abc", "a1b", "my-work", "web-2", "a-b-c", strings.Repeat("a", MaxLen)} {
		if !Valid(good) {
			t.Fatal("rejected a legal alias", good)
		}
	}
	for _, bad := range []string{"ab", strings.Repeat("a", MaxLen+1), "ABC", "123", "1abc", "my_work", "a__", "-abc", "abc-", "a--b", "a b", "한글", ""} {
		if Valid(bad) {
			t.Fatal("accepted an illegal alias", bad)
		}
	}
}

// Valid may only accept what the resource layer stores unchanged. Ask payday's
// grammar itself rather than restating it here: a rule that reads better but
// stores worse is a rename that fails after the user was told it succeeded.
func TestAliasSurvivesTheResourceLayer(t *testing.T) {
	for _, s := range []string{
		"abc", "a1b", "my-work", "web-2", "a-b-c", strings.Repeat("a", MaxLen),
		"ABC", "Abc", "my_work", "a__", "abc-", "a--b", "-abc", "ab",
		"123", "1abc", "a b", " abc ", "한글", "",
	} {
		stored, err := slug.ParseAlias(s)
		if !Valid(s) {
			continue // Stricter than the storage grammar is always safe.
		}
		if err != nil {
			t.Fatalf("Valid(%q) but the resource layer rejects it: %v", s, err)
		}
		if stored != s {
			t.Fatalf("Valid(%q) but the resource layer stores %q", s, stored)
		}
	}
	for _, word := range Words {
		if stored, err := slug.ParseAlias(word); err != nil || stored != word {
			t.Fatalf("generated alias %q does not survive storage: %q, %v", word, stored, err)
		}
	}
}

// resourceclient.sr tells an alias from a runtime ID by asking Valid. Every
// character of a core.ID is also legal in an alias, so only the length
// separates them: a wider MaxLen would route a session ID that happens to
// begin with a–f to the alias index instead.
func TestRuntimeIDIsNeverAnAlias(t *testing.T) {
	if MaxLen >= 24 {
		t.Fatal("MaxLen no longer excludes a 24-character runtime ID", MaxLen)
	}
	if Valid("abc123def456abc123def456") {
		t.Fatal("runtime ID accepted as an alias")
	}
}
