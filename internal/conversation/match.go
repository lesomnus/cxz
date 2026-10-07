package conversation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Matching conversation text is its own concern, apart from where the text
// comes from. A store that holds the conversation and a reader that walks a
// journal ask the same question of a string, and a person expects the same
// answer from both.
const (
	// DefaultSnippet is enough to read the matching sentence, not the message.
	DefaultSnippet = 200
	MaxSnippet     = 2048
)

// Match is how a query is compared against text.
type Match struct {
	Query      string
	Mode       string // substring (default), regex (RE2) or fuzzy (per line)
	IgnoreCase bool
}

type Matcher struct {
	re         *regexp.Regexp
	query      string
	fuzzy      bool
	ignoreCase bool
}

func NewMatcher(m Match) (*Matcher, error) {
	switch m.Mode {
	case "", "substring", "regex", "fuzzy":
	default:
		return nil, fmt.Errorf("match must be substring, regex or fuzzy")
	}
	out := &Matcher{query: m.Query, fuzzy: m.Mode == "fuzzy", ignoreCase: m.IgnoreCase}
	if m.Mode == "regex" {
		pattern := m.Query
		if m.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid RE2 expression: %w", err)
		}
		out.re = re
	}
	if out.ignoreCase {
		out.query = strings.ToLower(out.query)
	}
	return out, nil
}

// Find reports the score and the byte span that matched, which is what a
// snippet is cut around. An empty substring query matches everything, so a
// time range on its own is a valid question.
func (m *Matcher) Find(text string) (score, lo, hi int, ok bool) {
	switch {
	case m.re != nil:
		span := m.re.FindStringIndex(text)
		if span == nil {
			return 0, 0, 0, false
		}
		return 0, span[0], span[1], true
	case m.fuzzy:
		return fuzzyMatch(text, m.query, m.ignoreCase)
	default:
		if m.query == "" {
			return 0, 0, 0, true
		}
		haystack := text
		if m.ignoreCase {
			haystack = strings.ToLower(haystack)
		}
		i := strings.Index(haystack, m.query)
		if i < 0 {
			return 0, 0, 0, false
		}
		return 0, i, i + len(m.query), true
	}
}

// Snippet is the matching text with enough either side to read it as a
// sentence. Whitespace is collapsed because a transcript is full of it and a
// result list has one line.
func Snippet(text string, lo, hi, budget int) string {
	if budget <= 0 || text == "" {
		return ""
	}
	lo = min(max(lo, 0), len(text))
	hi = min(max(hi, lo), len(text))
	span := hi - lo
	if span > budget {
		hi = lo + budget
		span = budget
	}
	slack := budget - span
	start := lo - slack/2
	if start < 0 {
		start = 0
	}
	end := min(len(text), start+budget)
	start = max(0, min(start, end-budget))
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	var b strings.Builder
	if start > 0 {
		b.WriteRune('…')
	}
	space := false
	for _, r := range text[start:end] {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	if end < len(text) {
		b.WriteRune('…')
	}
	return b.String()
}
