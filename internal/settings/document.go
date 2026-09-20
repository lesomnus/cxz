package settings

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

//go:embed template.jsonc
var initialDocument string

// Template contains disabled examples and a local schema reference. All runtime
// preferences retain their defaults.
func Template() []byte { return []byte(initialDocument) }

// WithSchema adds the bundled schema reference while preserving comments and
// any existing custom schema reference.
func WithSchema(original []byte) ([]byte, error) {
	before, err := Parse(original)
	if err != nil {
		return nil, err
	}
	after := before
	if after.Schema == "" {
		after.Schema = SchemaReference
	}
	return updateDocument(original, before, after)
}

// Update only changed top-level settings. Unrelated subtrees retain their
// comments; comments around a removed preference are kept beside the next one.
func updateDocument(original []byte, before, after Config) ([]byte, error) {
	// Formatting an AST can reuse its input buffer. Keep the caller's original
	// bytes intact for the editor's concurrent-change check.
	doc, err := hujson.Parse(bytes.Clone(original))
	if err != nil {
		return nil, err
	}
	oldBytes, err := json.Marshal(before)
	if err != nil {
		return nil, err
	}
	newBytes, err := json.Marshal(after)
	if err != nil {
		return nil, err
	}
	oldValues, newValues := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if err = json.Unmarshal(oldBytes, &oldValues); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(newBytes, &newValues); err != nil {
		return nil, err
	}
	obj := doc.Value.(*hujson.Object) // Parse has already validated the settings object.
	changed := false
	names := make([]string, 0, len(oldValues)+len(newValues))
	for name := range oldValues {
		names = append(names, name)
	}
	for name := range newValues {
		if _, ok := oldValues[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if bytes.Equal(oldValues[name], newValues[name]) {
			continue
		}
		changed = true
		raw, present := newValues[name]
		var value hujson.Value
		if present {
			value, err = hujson.Parse(raw)
			if err != nil {
				return nil, err
			}
		}
		found := false
		for i := 0; i < len(obj.Members); {
			m := &obj.Members[i]
			if !strings.EqualFold(m.Name.Value.(hujson.Literal).String(), name) {
				i++
				continue
			}
			found = true
			if present {
				m.Value.Value = value.Clone().Value
				i++
				continue
			}
			// Keep explanation/example comments even when the preference is removed.
			extra := append(hujson.Extra{}, m.Name.BeforeExtra...)
			extra = append(extra, m.Name.AfterExtra...)
			extra = append(extra, m.Value.BeforeExtra...)
			extra = append(extra, m.Value.AfterExtra...)
			obj.Members = append(obj.Members[:i], obj.Members[i+1:]...)
			if i < len(obj.Members) {
				obj.Members[i].Name.BeforeExtra = append(extra, obj.Members[i].Name.BeforeExtra...)
			} else {
				obj.AfterExtra = append(extra, obj.AfterExtra...)
			}
		}
		if !found && present {
			obj.Members = append(obj.Members, hujson.ObjectMember{Name: hujson.Value{Value: hujson.String(name)}, Value: value})
		}
	}
	if !changed {
		return original, nil
	}
	doc.Format()
	return doc.Pack(), nil
}
