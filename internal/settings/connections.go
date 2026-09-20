package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lesomnus/cxz/internal/transport"
)

type Connection struct {
	Target    string `json:"target"`
	TokenFile string `json:"token_file,omitempty"`
}
type Connections struct {
	Default string
	Entries map[string]Connection
}

func (c *Connections) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*c = Connections{Entries: map[string]Connection{}}
	for name, value := range raw {
		if name == "default" {
			if err := json.Unmarshal(value, &c.Default); err != nil {
				return err
			}
			continue
		}
		var entry Connection
		d := json.NewDecoder(bytes.NewReader(value))
		d.DisallowUnknownFields()
		if err := d.Decode(&entry); err != nil {
			return fmt.Errorf("connection %s: %w", name, err)
		}
		c.Entries[name] = entry
	}
	return nil
}
func (c Connections) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if c.Default != "" {
		out["default"] = c.Default
	}
	for name, v := range c.Entries {
		out[name] = v
	}
	return json.Marshal(out)
}

var connectionName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,39}$`)

func (c Connections) Validate() error {
	if len(c.Entries) == 0 {
		return fmt.Errorf("connections must contain at least one named connection")
	}
	if c.Default != "" {
		if _, ok := c.Entries[c.Default]; !ok {
			return fmt.Errorf("default connection %q is not configured", c.Default)
		}
	}
	for name, v := range c.Entries {
		if name == "default" || !connectionName.MatchString(name) {
			return fmt.Errorf("invalid connection name %q", name)
		}
		target := strings.ReplaceAll(v.Target, "${STATE}", "/client-state")
		if strings.Contains(target, "${") || strings.Contains(strings.ReplaceAll(v.TokenFile, "${STATE}", ""), "${") {
			return fmt.Errorf("connection %s: only ${STATE} is supported", name)
		}
		e, err := transport.ParseEndpoint(target)
		if err != nil {
			return fmt.Errorf("connection %s: %w", name, err)
		}
		if e.Scheme == "tcp" && v.TokenFile == "" {
			return fmt.Errorf("connection %s requires token_file", name)
		}
		if e.Scheme != "tcp" && v.TokenFile != "" {
			return fmt.Errorf("connection %s: token_file is only used for TCP", name)
		}
	}
	return nil
}
func (c Connections) Names() []string {
	names := make([]string, 0, len(c.Entries))
	for name := range c.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (c Connections) DefaultName() string {
	if c.Default != "" {
		return c.Default
	}
	names := c.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}
func (c Connection) Resolve(state string) Connection {
	c.Target = strings.ReplaceAll(c.Target, "${STATE}", (&url.URL{Path: filepath.ToSlash(state)}).EscapedPath())
	c.TokenFile = strings.ReplaceAll(c.TokenFile, "${STATE}", state)
	if c.TokenFile != "" && !filepath.IsAbs(c.TokenFile) {
		c.TokenFile = filepath.Join(state, c.TokenFile)
	}
	return c
}
