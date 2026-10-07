package conversation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fuzzy matching here is fzf's rule, scoped to one line.
//
// Scoping matters more than scoring. A message is paragraphs long, and a
// query's letters can always be found scattered across one, so a fuzzy match
// against a whole message means nothing: it finds every message. Against a
// line it means what a person means by it -- a typo, a missing word, a half
// remembered phrase -- and the span it matches is short enough to show.
const (
	fuzzyMatchScore  = 16 // one query rune in place
	fuzzyAdjacent    = 8  // the previous query rune matched the previous rune
	fuzzyBoundary    = 8  // a word starts here
	fuzzyGapPenalty  = 1  // per rune skipped inside the span
	fuzzyMinSpan     = 32 // a short query may still wander this far
	fuzzySpanPerRune = 4
)

// fuzzyMatch finds the best line and returns its span in text's bytes.
func fuzzyMatch(text, query string, ignoreCase bool) (score, lo, hi int, ok bool) {
	best := 0
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if s, a, b, found := fuzzyLine(line, query, ignoreCase); found && (!ok || s > best) {
			best, lo, hi, ok = s, offset+a, offset+b, true
		}
		offset += len(line)
	}
	return best, lo, hi, ok
}

// fuzzyLine matches forwards to find the end of the shortest useful span, then
// backwards to tighten its start -- the two passes are what make "cxz" prefer
// "cxz web" over a c, an x and a z strewn across a sentence.
func fuzzyLine(line, query string, ignoreCase bool) (score, lo, hi int, ok bool) {
	qr := []rune(query)
	if len(qr) == 0 {
		return 0, 0, 0, false
	}
	limit := max(fuzzyMinSpan, fuzzySpanPerRune*len(qr))
	qi := 0
	end := -1
	for i, r := range line {
		if fuzzyEqual(r, qr[qi], ignoreCase) {
			qi++
			if qi == len(qr) {
				end = i + utf8.RuneLen(r)
				break
			}
		}
	}
	if end < 0 {
		return 0, 0, 0, false
	}
	// Tighten: the last query rune is at the end, so matching the query
	// backwards from there finds the latest possible start.
	qi = len(qr) - 1
	start := -1
	for i := end; i > 0; {
		r, size := utf8.DecodeLastRuneInString(line[:i])
		i -= size
		if fuzzyEqual(r, qr[qi], ignoreCase) {
			if qi == 0 {
				start = i
				break
			}
			qi--
		}
	}
	if start < 0 {
		return 0, 0, 0, false
	}
	span := utf8.RuneCountInString(line[start:end])
	if span > limit {
		return 0, 0, 0, false
	}
	return fuzzyScore(line, start, end, qr, ignoreCase), start, end, true
}

func fuzzyScore(line string, start, end int, qr []rune, ignoreCase bool) int {
	score := 0
	qi := 0
	previous := -1
	before, _ := utf8.DecodeLastRuneInString(line[:start])
	for i, r := range line[start:end] {
		if qi < len(qr) && fuzzyEqual(r, qr[qi], ignoreCase) {
			score += fuzzyMatchScore
			if previous == i-utf8.RuneLen(r) {
				score += fuzzyAdjacent
			}
			if i == 0 && (start == 0 || !unicode.IsLetter(before) && !unicode.IsDigit(before)) {
				score += fuzzyBoundary
			} else if i > 0 {
				if prev, size := utf8.DecodeLastRuneInString(line[start : start+i]); size > 0 && !unicode.IsLetter(prev) && !unicode.IsDigit(prev) {
					score += fuzzyBoundary
				}
			}
			previous = i
			qi++
			continue
		}
		score -= fuzzyGapPenalty
	}
	return max(score, 1)
}

func fuzzyEqual(a, b rune, ignoreCase bool) bool {
	if a == b {
		return true
	}
	return ignoreCase && unicode.ToLower(a) == unicode.ToLower(b)
}
