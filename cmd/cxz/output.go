//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
	"github.com/lesomnus/xli/tab"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type formatParser struct{ flg.StringParser }

func (formatParser) Parse(v string) (string, error) {
	if v != "table" && v != "json" {
		return "", fmt.Errorf("format must be table or json")
	}
	return v, nil
}
func formatFlag() *flg.Base[string, formatParser] {
	f := &flg.Base[string, formatParser]{Name: "format", Brief: "Output format: table or json (JSON preserves all fields)", Default: ptr("table")}
	f.Handler = flg.OnTab[string](func(_ context.Context, t tab.Tab) error { t.Value("table"); t.Value("json"); return nil })
	return f
}
func writeJSON(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }
func writeResource(c *xli.Command, v proto.Message) error {
	b, err := protojson.Marshal(v)
	if err != nil {
		return err
	}
	return renderOutput(c, b, false)
}
func writeOutput(c *xli.Command, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, event := v.(*api.Event)
	return renderOutput(c, b, event)
}
func renderOutput(c *xli.Command, b []byte, event bool) error {
	format := "table"
	for cur := c; cur != nil; {
		if v, set := flg.Get[string](cur, "format"); set {
			format = v
			break
		}
		if !cur.HasParent() {
			break
		}
		cur = cur.Parent()
	}
	if format == "json" {
		_, err := fmt.Fprintln(c.Writer, string(b))
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if m, ok := value.(map[string]any); ok && c.Name == "ls" {
		for _, key := range []string{"items", "sessions", "projects"} {
			if list, exists := m[key]; exists {
				value = list
				if list == nil {
					value = []any{}
				}
				break
			}
		}
		if len(m) == 0 && c.Name == "ls" {
			value = []any{}
		}
	}
	if event {
		value = []any{value}
	}
	if list, ok := value.([]any); ok {
		if len(list) == 0 {
			_, err := fmt.Fprintln(c.Writer, "No results.")
			return err
		}
		if c.HasParent() && c.Parent().Name == "backend" {
			var flat []any
			for _, item := range list {
				a, ok := item.(map[string]any)
				if !ok {
					continue
				}
				backends, _ := a["backends"].([]any)
				for _, raw := range backends {
					if b, ok := raw.(map[string]any); ok {
						flat = append(flat, map[string]any{"agent": a["kind"], "backend": b["id"], "default": b["id"] == a["default_backend"], "scope": b["scope"], "workflow": b["workflow"]})
					}
				}
			}
			list = flat
			if len(list) == 0 {
				_, err := fmt.Fprintln(c.Writer, "No results.")
				return err
			}
		}
		preferred := []string{"id", "alias", "name", "agent", "account", "state", "workspace"}
		if c.HasParent() {
			switch c.Parent().Name {
			case "session":
				preferred = []string{"alias", "id", "title", "agent", "account", "state"}
			case "account":
				preferred = []string{"alias", "name", "agent", "authBackend"}
			case "binding":
				preferred = []string{"bindingId", "account", "project", "authBackend", "scope"}
			case "backend":
				preferred = []string{"agent", "backend", "default", "scope", "workflow"}
			case "manager":
				preferred = []string{"name", "ok", "detail"}
			}
		}
		if event {
			preferred = []string{"seq", "kind", "text", "request_id"}
		}
		keys := []string{}
		for _, key := range preferred {
			for _, row := range list {
				if m, ok := row.(map[string]any); ok {
					if _, exists := m[key]; exists {
						keys = append(keys, key)
						break
					}
				}
			}
		}
		if len(keys) == 0 {
			if m, ok := list[0].(map[string]any); ok {
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
			}
		}
		if len(keys) == 0 {
			keys = []string{"value"}
		}
		rows := [][]string{}
		for _, row := range list {
			cells := make([]string, len(keys))
			m, ok := row.(map[string]any)
			for i, key := range keys {
				if ok {
					cells[i] = tableCell(m[key])
				} else {
					cells[i] = tableCell(row)
				}
			}
			rows = append(rows, cells)
		}
		return writeTable(c.Writer, keys, rows)
	}
	if m, ok := value.(map[string]any); ok {
		keys := []string{}
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := [][]string{}
		for _, k := range keys {
			rows = append(rows, []string{k, tableCell(m[k])})
		}
		return writeTable(c.Writer, []string{"field", "value"}, rows)
	}
	return writeTable(c.Writer, []string{"value"}, [][]string{{tableCell(value)}})
}
func tableCell(v any) string {
	if v == nil {
		return "-"
	}
	s, ok := v.(string)
	if !ok {
		b, _ := json.Marshal(v)
		s = string(b)
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if s == "" {
		return "-"
	}
	return ansi.Truncate(s, 72, "…")
}
func writeTable(w io.Writer, keys []string, rows [][]string) error {
	header := make([]string, len(keys))
	widths := make([]int, len(keys))
	for i, k := range keys {
		header[i] = strings.ToUpper(k)
		widths[i] = ansi.StringWidth(header[i])
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}
	limit := 120
	if f, ok := w.(term.File); ok {
		if width, _, err := term.GetSize(f.Fd()); err == nil && width > 0 {
			limit = width
		}
	}
	for {
		total := max(0, len(keys)-1) * 2
		largest := -1
		for i, width := range widths {
			total += width
			if width > 4 && (largest < 0 || width > widths[largest]) {
				largest = i
			}
		}
		if total <= limit || largest < 0 {
			break
		}
		widths[largest]--
	}
	for _, row := range append([][]string{header}, rows...) {
		for i, cell := range row {
			cell = ansi.Truncate(cell, widths[i], "…")
			if _, err := io.WriteString(w, cell); err != nil {
				return err
			}
			if i < len(row)-1 {
				if _, err := io.WriteString(w, strings.Repeat(" ", widths[i]-ansi.StringWidth(cell)+2)); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}
