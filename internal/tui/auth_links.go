package tui

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var authURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// Wrap the visible label, never the hyperlink target or clipboard source.
// Provider escapes are discarded; only our own validated OSC 8 links survive.
// A URL must have a terminating delimiter before becoming actionable: output
// arrives in arbitrary chunks, including in the middle of query parameters.
func workflowAuthOutput(raw string, width int) ([]string, string) {
	text := safeText(ansi.Strip(raw))
	var rows []string
	latest := ""
	plain := func(s string) { rows = append(rows, strings.Split(ansi.Hardwrap(s, max(1, width), true), "\n")...) }
	lines := strings.Split(text, "\n")
	for n, line := range lines {
		matches := authURLPattern.FindAllStringIndex(line, -1)
		pos := 0
		for _, at := range matches {
			link := line[at[0]:at[1]]
			u, err := url.Parse(link)
			complete := at[1] < len(line) || n < len(lines)-1
			if err != nil || u.Hostname() == "" || u.User != nil || !complete {
				continue
			}
			if prefix := strings.TrimSpace(line[pos:at[0]]); prefix != "" {
				plain(prefix)
			}
			for _, part := range strings.Split(ansi.Hardwrap(link, max(1, width), true), "\n") {
				rows = append(rows, ansi.SetHyperlink(link)+part+ansi.ResetHyperlink())
			}
			latest = link
			pos = at[1]
		}
		if pos == 0 {
			plain(line)
		} else if suffix := strings.TrimSpace(line[pos:]); suffix != "" {
			plain(suffix)
		}
	}
	return rows, latest
}
