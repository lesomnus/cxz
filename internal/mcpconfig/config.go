// Package mcpconfig stores manager-owned MCP registrations and project overrides.
package mcpconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/gofrs/flock"
	"github.com/lesomnus/cxz/internal/core"
)

type Server struct {
	Name    string            `json:"name"`
	Kind    string            `json:"kind"` // stdio, http, builtin
	Enabled bool              `json:"enabled"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"` // explicit values; stored privately
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}
type Config struct {
	Servers  map[string]Server          `json:"servers"`
	Projects map[string]map[string]bool `json:"projects,omitempty"`
}
type Entry struct {
	ID     string
	Server Server
	// Set when this project decided for itself rather than inheriting.
	Override  *bool
	Effective bool
}

// Listing is the registrations as one scope sees them. Every operation returns
// one, because a change to activation is only legible next to what it changed.
// Entries never carry environment values or headers: see View.
type Listing struct {
	Entries []Entry
	Message string
}
type Snapshot struct {
	Servers map[string]Server `json:"servers"`
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var builtins = map[string]Server{}

func RegisterBuiltin(id, name string) {
	builtins[id] = Server{Name: name, Kind: "builtin", Enabled: true}
}
func Builtin(id string) bool { _, ok := builtins[id]; return ok }
func ValidateID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid MCP ID")
	}
	return nil
}
func (s Server) Validate() error {
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 100 || strings.IndexFunc(s.Name, unicode.IsControl) >= 0 {
		return fmt.Errorf("MCP name must contain 1–100 bytes")
	}
	switch s.Kind {
	case "stdio":
		if strings.TrimSpace(s.Command) == "" || strings.ContainsAny(s.Command, "\x00\r\n") || s.URL != "" || len(s.Headers) > 0 {
			return fmt.Errorf("stdio requires a command, without URL or headers")
		}
	case "http":
		u, e := url.Parse(s.URL)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" || s.Command != "" || len(s.Args) > 0 || len(s.Env) > 0 {
			return fmt.Errorf("HTTP requires an http(s) URL without command, environment, userinfo or fragment")
		}
	case "builtin":
		if s.Command != "" || s.URL != "" || len(s.Args) > 0 || len(s.Env) > 0 || len(s.Headers) > 0 {
			return fmt.Errorf("built-in MCP cannot have external configuration")
		}
	default:
		return fmt.Errorf("MCP kind must be stdio or http")
	}
	for k, v := range s.Env {
		if k == "" || strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) {
			return fmt.Errorf("invalid MCP environment")
		}
	}
	for k, v := range s.Headers {
		if k == "" || strings.ContainsAny(k, "\r\n :\t") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid MCP HTTP header")
		}
	}
	for _, a := range s.Args {
		if strings.ContainsRune(a, 0) {
			return fmt.Errorf("invalid MCP argument")
		}
	}
	return nil
}
func Load(root string) (Config, error) {
	c := Config{Servers: map[string]Server{}, Projects: map[string]map[string]bool{}}
	b, e := os.ReadFile(filepath.Join(root, "mcp.json"))
	if e != nil && !os.IsNotExist(e) {
		return c, e
	}
	if e == nil {
		if e = json.Unmarshal(b, &c); e != nil {
			return c, e
		}
	}
	if c.Servers == nil {
		c.Servers = map[string]Server{}
	}
	if c.Projects == nil {
		c.Projects = map[string]map[string]bool{}
	}
	for id, s := range builtins {
		if _, ok := c.Servers[id]; !ok {
			c.Servers[id] = s
		}
	}
	for id, s := range c.Servers {
		if err := ValidateID(id); err != nil {
			return c, err
		}
		if err := s.Validate(); err != nil {
			return c, fmt.Errorf("MCP %s: %w", id, err)
		}
		if s.Kind == "builtin" && !Builtin(id) {
			return c, fmt.Errorf("unknown built-in MCP %s", id)
		}
		if Builtin(id) && s.Kind != "builtin" {
			return c, fmt.Errorf("reserved built-in MCP %s", id)
		}
	}
	return c, nil
}
func (c Config) Resolve(project string) Snapshot {
	out := Snapshot{Servers: map[string]Server{}}
	for id, s := range c.Servers {
		enabled := s.Enabled
		if v, ok := c.Projects[project][id]; ok {
			enabled = v
		}
		if enabled {
			s.Enabled = true
			out.Servers[id] = s
		}
	}
	return out
}

// View strips the environment and the headers out of every entry. They are
// what the caller wrote and never what it needs read back, and a list is the
// one MCP reply that goes to every client that asks.
func (c Config) View(project string) Listing {
	r := Listing{}
	for id, s := range c.Servers {
		v := Entry{ID: id, Server: s, Effective: s.Enabled}
		if b, ok := c.Projects[project][id]; ok {
			v.Override = &b
			v.Effective = b
		}
		v.Server.Env = nil
		v.Server.Headers = nil
		r.Entries = append(r.Entries, v)
	}
	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].ID < r.Entries[j].ID })
	return r
}

// List reports the registrations without changing them. project chooses whose
// view: empty is the installation's own defaults.
func List(root, project string) (Listing, error) {
	return read(root, func(c Config) (Listing, error) { return c.View(project), nil })
}

// Put registers or replaces an external MCP. There is no project to name,
// because a registration is the installation's; which projects see it is a
// different decision with its own calls.
func Put(root, id string, server Server) (Listing, error) {
	return edit(root, "", id, func(c *Config) error {
		if Builtin(id) || server.Kind == "builtin" {
			return fmt.Errorf("built-in MCP cannot be replaced")
		}
		if e := server.Validate(); e != nil {
			return e
		}
		c.Servers[id] = server
		return nil
	})
}

func Remove(root, id string) (Listing, error) {
	return edit(root, "", id, func(c *Config) error {
		if _, ok := c.Servers[id]; !ok || Builtin(id) {
			return fmt.Errorf("external global MCP required")
		}
		delete(c.Servers, id)
		for _, p := range c.Projects {
			delete(p, id)
		}
		return nil
	})
}

// SetDefault decides what a project sees when it has not decided for itself.
func SetDefault(root, id string, enabled bool) (Listing, error) {
	return edit(root, "", id, func(c *Config) error {
		s, ok := c.Servers[id]
		if !ok {
			return fmt.Errorf("unknown MCP server")
		}
		s.Enabled = enabled
		c.Servers[id] = s
		return nil
	})
}

// SetProject decides for one project, overriding the default.
func SetProject(root, project, id string, enabled bool) (Listing, error) {
	if project == "" {
		return Listing{}, fmt.Errorf("a project activation needs a project")
	}
	return edit(root, project, id, func(c *Config) error {
		if _, ok := c.Servers[id]; !ok {
			return fmt.Errorf("unknown MCP server")
		}
		if c.Projects[project] == nil {
			c.Projects[project] = map[string]bool{}
		}
		c.Projects[project][id] = enabled
		return nil
	})
}

// ClearProject drops a project's own decision so it inherits again. There is
// nowhere here to put an on or an off, which is the point of it being its own
// call: nil used to mean this, in the same field that otherwise meant a value.
func ClearProject(root, project, id string) (Listing, error) {
	if project == "" {
		return Listing{}, fmt.Errorf("inherit needs a project to restore")
	}
	return edit(root, project, id, func(c *Config) error {
		if _, ok := c.Servers[id]; !ok {
			return fmt.Errorf("unknown MCP server")
		}
		delete(c.Projects[project], id)
		return nil
	})
}

// read holds the lock for a read too, so a list cannot observe half of a write.
func read(root string, view func(Config) (Listing, error)) (Listing, error) {
	unlock, e := lock(root)
	if e != nil {
		return Listing{}, e
	}
	defer unlock()
	c, e := Load(root)
	if e != nil {
		return Listing{}, e
	}
	return view(c)
}

// edit is the part every change shares: a valid id, one read and one write
// under the lock, and the view that comes back is the one the caller asked for
// rather than the one the change happened to touch.
func edit(root, project, id string, change func(*Config) error) (Listing, error) {
	if e := ValidateID(id); e != nil {
		return Listing{}, e
	}
	unlock, e := lock(root)
	if e != nil {
		return Listing{}, e
	}
	defer unlock()
	c, e := Load(root)
	if e != nil {
		return Listing{}, e
	}
	if e = change(&c); e != nil {
		return Listing{}, e
	}
	if e = core.WriteJSON(filepath.Join(root, "mcp.json"), c); e != nil {
		return Listing{}, e
	}
	out := c.View(project)
	out.Message = SavedMessage
	return out, nil
}

// SavedMessage says what a change does and does not do: settings land at the
// next agent launch, and a working agent is never restarted under someone.
const SavedMessage = "Saved. New or restarted agents use these settings; running agents keep their launch configuration."

func lock(root string) (func(), error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	l := flock.New(filepath.Join(root, "mcp.lock"))
	if e := l.Lock(); e != nil {
		return nil, e
	}
	return func() { l.Unlock() }, nil
}

func (s Snapshot) Digest() string {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func RuntimePath(root string) string { return filepath.Join(root, "mcp-runtime.json") }
func LoadRuntime(root string) (Snapshot, error) {
	s := Snapshot{Servers: map[string]Server{}}
	b, e := os.ReadFile(RuntimePath(root))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func SaveRuntime(root string, s Snapshot) error {
	for id, v := range s.Servers {
		if e := ValidateID(id); e != nil {
			return e
		}
		if e := v.Validate(); e != nil {
			return e
		}
	}
	return core.WriteJSON(RuntimePath(root), s)
}

// ForgetProject removes overrides without modifying global server definitions.
func ForgetProject(root, project string) error {
	l := flock.New(filepath.Join(root, "mcp.lock"))
	if err := l.Lock(); err != nil {
		return err
	}
	defer l.Unlock()
	c, err := Load(root)
	if err != nil {
		return err
	}
	if _, ok := c.Projects[project]; !ok {
		return nil
	}
	delete(c.Projects, project)
	return core.WriteJSON(filepath.Join(root, "mcp.json"), c)
}
