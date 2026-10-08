package shellview

import (
	"path"
	"strings"

	"github.com/mattn/go-shellwords"
)

// Codex hands a script to a login shell -- /usr/bin/zsh -lc "rg …; git log …" --
// so the wrapper is the first thing on the row, and the script it carries is
// highlighted as one quoted string rather than as the commands it contains. Name
// the shell separately and let Command be the script, which is what a reader is
// looking for. The original stays in the raw payload the tool preview shows.
//
// Only a lone trailing operand qualifies: with a second one the first is $0 and
// no longer the whole script. Only a short option bundle is read for c, because
// a long option never means "run this string" -- --no-rcs must not look like one.
func Unwrap(command string) (string, string) {
	words, err := shellwords.NewParser().Parse(command)
	if err != nil || len(words) < 3 {
		return "", command
	}
	shell := path.Base(words[0])
	switch shell {
	case "sh", "bash", "zsh", "dash", "ksh", "ash":
	default:
		return "", command
	}
	carries := false
	for i, options := 0, words[1:len(words)-1]; i < len(options); i++ {
		w := options[i]
		switch {
		case w == "-c":
			carries = true
		case w == "-o" || w == "+o":
			i++ // Its value is the option's, not a second operand.
		case strings.HasPrefix(w, "--"):
		case strings.HasPrefix(w, "-"):
			carries = carries || strings.ContainsRune(w[1:], 'c')
		default:
			return "", command
		}
	}
	if script := words[len(words)-1]; carries && strings.TrimSpace(script) != "" {
		return shell, script
	}
	return "", command
}
