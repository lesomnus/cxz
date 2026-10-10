// Package sessiontitle defines the normalization shared by manual and generated titles.
package sessiontitle

import (
	"strings"
	"unicode"
)

const MaxInputBytes = 4096

func Normalize(text string) string {
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	text = strings.Trim(text, "\"'` ")
	if runes := []rune(text); len(runes) > 120 {
		text = string(runes[:117]) + "..."
	}
	return text
}
