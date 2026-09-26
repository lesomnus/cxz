package sessionalias

import (
	"strings"
	"testing"
)

func TestWords(t *testing.T) {
	seen := map[string]bool{}
	for _, word := range Words {
		if !Valid(word) || seen[word] {
			t.Fatal("invalid or duplicate word", word)
		}
		seen[word] = true
	}
	for _, good := range []string{"ABC", "my_work", "abc-", "a--b", "a__", "abc", "a1b", "my-work", "web-2", "a-b-c", strings.Repeat("a", MaxLen)} {
		if !Valid(good) {
			t.Fatal("rejected a legal alias", good)
		}
	}
	for _, bad := range []string{"ab", strings.Repeat("a", MaxLen+1), "123", "1abc", "-abc", "a b", "한글", ""} {
		if Valid(bad) {
			t.Fatal("accepted an illegal alias", bad)
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
