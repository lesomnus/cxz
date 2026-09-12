package sessionalias

import "testing"

func TestWords(t *testing.T) {
	seen := map[string]bool{}
	for _, word := range Words {
		if !Valid(word) || seen[word] {
			t.Fatal("invalid or duplicate word", word)
		}
		seen[word] = true
	}
	for _, bad := range []string{"ab", "abcdefgh", "ABC", "a-b", "123", "한글"} {
		if Valid(bad) {
			t.Fatal(bad)
		}
	}
}
