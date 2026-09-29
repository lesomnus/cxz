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
type Request struct {
	Session string  `json:"session,omitempty"`
	Action  string  `json:"action"`
	Project string  `json:"project,omitempty"`
	ID      string  `json:"id,omitempty"`
	Server  *Server `json:"server,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"` // nil restores inheritance
}
type Entry struct {
	ID        string `json:"id"`
	Server    Server `json:"server"`
	Override  *bool  `json:"override,omitempty"`
	Effective bool   `json:"effective"`
}
type SessionStatus struct {
	LaunchDigest string            `json:"launch_digest,omitempty"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Pending      bool              `json:"pending"`
	Servers      map[string]string `json:"servers"`
}
type Reply struct {
	Log      string          `json:"log,omitempty"`
	Sessions []SessionStatus `json:"sessions,omitempty"`
	Entries  []Entry         `json:"entries"`
	Message  string          `json:"message,omitempty"`
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
func (c Config) View(project string) Reply {
	r := Reply{}
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
func Apply(root string, r Request) (Reply, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return Reply{}, e
	}
	l := flock.New(filepath.Join(root, "mcp.lock"))
	if e := l.Lock(); e != nil {
		return Reply{}, e
	}
	defer l.Unlock()
	c, e := Load(root)
	if e != nil {
		return Reply{}, e
	}
	if r.Action == "" || r.Action == "list" {
		return c.View(r.Project), nil
	}
	if e = ValidateID(r.ID); e != nil {
		return Reply{}, e
	}
	s, exists := c.Servers[r.ID]
	switch r.Action {
	case "put":
		if r.Project != "" || r.Server == nil {
			return Reply{}, fmt.Errorf("register servers globally")
		}
		if Builtin(r.ID) || r.Server.Kind == "builtin" {
			return Reply{}, fmt.Errorf("built-in MCP cannot be replaced")
		}
		if e = r.Server.Validate(); e != nil {
			return Reply{}, e
		}
		c.Servers[r.ID] = *r.Server
	case "remove":
		if !exists || Builtin(r.ID) || r.Project != "" {
			return Reply{}, fmt.Errorf("external global MCP required")
		}
		delete(c.Servers, r.ID)
		for _, p := range c.Projects {
			delete(p, r.ID)
		}
	case "enable":
		if !exists {
			return Reply{}, fmt.Errorf("unknown MCP server")
		}
		if r.Project == "" {
			if r.Enabled == nil {
				return Reply{}, fmt.Errorf("global default requires on or off")
			}
			s.Enabled = *r.Enabled
			c.Servers[r.ID] = s
		} else {
			if c.Projects[r.Project] == nil {
				c.Projects[r.Project] = map[string]bool{}
			}
			if r.Enabled == nil {
				delete(c.Projects[r.Project], r.ID)
			} else {
				c.Projects[r.Project][r.ID] = *r.Enabled
			}
		}
	default:
		return Reply{}, fmt.Errorf("unknown MCP action")
	}
	if e = core.WriteJSON(filepath.Join(root, "mcp.json"), c); e != nil {
		return Reply{}, e
	}
	out := c.View(r.Project)
	out.Message = "Saved. New or restarted agents use these settings; running agents keep their launch configuration."
	return out, nil
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
