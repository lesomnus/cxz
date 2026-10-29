// Package secretpaste classifies whole pasted tokens, never substrings.
package secretpaste

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Patterns are also imported by the browser. Keep expressions compatible with
// both Go and JavaScript; matching the entire trimmed input is enforced here.
//
//go:embed patterns.json
var patterns []byte

type rule struct {
	Name      string `json:"name"`
	Pattern   string `json:"pattern"`
	Validator string `json:"validator"`
	compiled  *regexp.Regexp
}

var rules = func() []rule {
	var result []rule
	if err := json.Unmarshal(patterns, &result); err != nil {
		panic(err)
	}
	for i := range result {
		result[i].compiled = regexp.MustCompile("^(?:" + result[i].Pattern + ")$")
		if result[i].Validator != "" && result[i].Validator != "jwt" {
			panic("unknown secret paste validator")
		}
	}
	return result
}()

// Detect returns a display name, or an empty string for ordinary pasted text.
// Recognition does not authenticate the token, verify its signature or check
// expiry. An expired credential should still take the secret storage path.
func Detect(text string) string {
	// Match JavaScript String.trim: Unicode whitespace plus BOM, excluding NEL.
	text = strings.TrimFunc(text, func(r rune) bool { return unicode.IsSpace(r) && r != '\u0085' || r == '\uFEFF' })
	for _, rule := range rules {
		if rule.compiled.MatchString(text) && (rule.Validator != "jwt" || isJWT(text)) {
			return rule.Name
		}
	}
	return ""
}

func isJWT(text string) bool {
	parts := strings.Split(text, ".")
	objects := make([]map[string]json.RawMessage, 2)
	for i := range objects {
		body, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil || !utf8.Valid(body) || json.Unmarshal(body, &objects[i]) != nil || objects[i] == nil {
			return false
		}
	}
	var alg string
	if json.Unmarshal(objects[0]["alg"], &alg) != nil || alg == "" {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(parts[2])
	return err == nil
}
