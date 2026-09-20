// Package logview builds bounded, read-only diagnostic reports.
package logview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lesomnus/cxz/internal/core"
)

const Limit = 1024 * 1024
const FileLimit = 64 * 1024

type Report struct{ strings.Builder }

func (r *Report) Add(name, body string) {
	if r.Len() >= Limit {
		return
	}
	text := strings.ToValidUTF8("\n── "+name+" ──\n"+body+"\n", "�")
	remaining := Limit - r.Len()
	if len(text) > remaining {
		marker := "\n[report size limit reached; remaining sources omitted]\n"
		n := max(0, remaining-len(marker))
		for n > 0 && !utf8.ValidString(text[:n]) {
			n--
		}
		if remaining < len(marker) {
			text = marker[:remaining]
		} else {
			text = text[:n] + marker
		}
	}
	r.WriteString(text)
}
func (r *Report) File(name, path string) {
	b, truncated, err := Tail(path, FileLimit)
	if err != nil {
		r.Add(name, "Unavailable: "+err.Error())
		return
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
		truncated = true
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(empty)"
	}
	if truncated {
		text = "[showing tail: at most 200 lines / 64 KiB]\n" + text
	}
	r.Add(name, strings.ToValidUTF8(text, "�"))
}
func (r *Report) Journal(name, path string) {
	b, truncated, err := Tail(path, Limit)
	if err != nil {
		r.Add(name, "Unavailable: "+err.Error())
		return
	}
	var lines []string
	records := bytes.Split(b, []byte{'\n'})
	// An unfinished final record is not committed and is never displayed.
	for _, record := range records[:max(0, len(records)-1)] {
		var events []core.Event
		if len(record) > 0 && record[0] == '[' {
			_ = json.Unmarshal(record, &events)
		} else {
			var event core.Event
			if json.Unmarshal(record, &event) == nil {
				events = []core.Event{event}
			}
		}
		for _, e := range events {
			switch e.Kind {
			case "diagnostic", "error", "update", "permission", "state", "setting_status", "usage_status":
				lines = append(lines, fmt.Sprintf("%s [%s] %s", time.UnixMilli(e.TimeMS).Format("01-02 15:04:05"), e.Kind, e.Text))
			}
		}
	}
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
		truncated = true
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no diagnostic events in the journal tail)"
	}
	if truncated {
		text = "[recent journal tail only: 1 MiB / 200 diagnostic events]\n" + text
	}
	if len(text) > FileLimit {
		text = text[:FileLimit] + "\n[diagnostic text truncated]"
	}
	r.Add(name, strings.ToValidUTF8(text, "�"))
}
