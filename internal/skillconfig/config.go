// Package skillconfig holds the installation's Agent Skills library and which
// projects see each skill. A skill is a directory with a SKILL.md, read by both
// supported agents from their own config directory, so cxz stores one copy and
// decides per project which ones are delivered.
//
// The library lives beside the other user-managed sources, under the share
// directory, so `cxz share` edits it and nothing has to be uploaded.
package skillconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/filemap"
)

// Codex installs its own skills into the same directory. Never take the name.
const Reserved = ".system"

const (
	MaxSkills      = 64
	MaxDescription = 1024
	maxSkillFile   = 256 * 1024
	filename       = "skills.json"
	runtimeFile    = "skills-runtime.json"
)

// The Agent Skills specification: lowercase alphanumerics and single hyphens,
// not leading or trailing, and the name must match its directory.
var validName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Skill struct {
	Description string `json:"description,omitempty"`
	// The default for projects that have not decided for themselves. New
	// skills start off: a name and description are loaded into every session's
	// context whether or not the skill is used.
	Enabled bool `json:"enabled"`
}

type Config struct {
	Skills   map[string]Skill           `json:"skills"`
	Projects map[string]map[string]bool `json:"projects,omitempty"`
}

type Request struct {
	Action  string `json:"action"`
	Project string `json:"project,omitempty"`
	Name    string `json:"name,omitempty"`
}

type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Set when this project decided for itself rather than inheriting.
	Override  *bool `json:"override,omitempty"`
	Effective bool  `json:"effective"`
}

type Reply struct {
	Entries []Entry `json:"entries"`
	Message string  `json:"message,omitempty"`
}

// Library is where the user keeps skill directories.
func Library(root string) string { return filepath.Join(filemap.ShareDir(root), "skills") }

func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("skill name required")
	case len(name) > 64:
		return fmt.Errorf("skill name exceeds 64 characters")
	case name == Reserved:
		return fmt.Errorf("%s is reserved for the provider's own skills", Reserved)
	case !validName.MatchString(name):
		return fmt.Errorf("skill name must be lowercase letters, digits and single hyphens: %s", name)
	}
	return nil
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Read reports what a skill directory declares about itself, and refuses what
// the specification refuses: the frontmatter's name has to match the directory
// that holds it, so a skill cannot be delivered under a name it does not answer
// to.
func Read(library, name string) (Skill, error) {
	if err := ValidateName(name); err != nil {
		return Skill{}, err
	}
	b, err := os.ReadFile(filepath.Join(library, name, "SKILL.md"))
	if os.IsNotExist(err) {
		return Skill{}, fmt.Errorf("%s has no SKILL.md", name)
	}
	if err != nil {
		return Skill{}, err
	}
	if len(b) > maxSkillFile {
		return Skill{}, fmt.Errorf("SKILL.md exceeds %d KiB", maxSkillFile/1024)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, fmt.Errorf("%s: SKILL.md must begin with YAML frontmatter", name)
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return Skill{}, fmt.Errorf("%s: SKILL.md frontmatter is not terminated", name)
	}
	var f frontmatter
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &f); err != nil {
		return Skill{}, fmt.Errorf("%s: %w", name, err)
	}
	if f.Name != name {
		return Skill{}, fmt.Errorf("%s: SKILL.md declares name %q, which is not its directory", name, f.Name)
	}
	if f.Description == "" {
		return Skill{}, fmt.Errorf("%s: SKILL.md needs a description", name)
	}
	if len(f.Description) > MaxDescription {
		return Skill{}, fmt.Errorf("%s: description exceeds %d characters", name, MaxDescription)
	}
	return Skill{Description: f.Description}, nil
}

func Load(root string) (Config, error) {
	c := Config{Skills: map[string]Skill{}}
	b, err := os.ReadFile(filepath.Join(root, filename))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Skills == nil {
		c.Skills = map[string]Skill{}
	}
	return c, nil
}

func save(root string, c Config) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return core.WriteJSON(filepath.Join(root, filename), c)
}

// Resolve names the skills a project receives. A project's own decision wins
// over the global default; with no decision it inherits.
func (c Config) Resolve(project string) []string {
	var out []string
	for name, s := range c.Skills {
		enabled := s.Enabled
		if v, ok := c.Projects[project][name]; ok {
			enabled = v
		}
		if enabled {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (c Config) View(project string) Reply {
	r := Reply{Entries: []Entry{}}
	for name, s := range c.Skills {
		e := Entry{Name: name, Description: s.Description, Effective: s.Enabled}
		if v, ok := c.Projects[project][name]; ok {
			e.Override = &v
			e.Effective = v
		}
		r.Entries = append(r.Entries, e)
	}
	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].Name < r.Entries[j].Name })
	return r
}

// Mappings describes where a project's skills are delivered. Both agents read
// their skills from their own configuration directory, so one destination
// serves either of them and no mapping needs an agent.
func (c Config) Mappings(project string) []filemap.Mapping {
	var out []filemap.Mapping
	for _, name := range c.Resolve(project) {
		out = append(out, filemap.Mapping{
			// Slash-joined on purpose: the destination is a container path and
			// the source is expanded by the manager, not by this machine.
			Src: filemap.ShareVariable + "/skills/" + name,
			Dst: "${AGENT_CONFIG_DIR}/skills/" + name,
		})
	}
	return out
}

func Apply(root string, r Request) (Reply, error) {
	c, err := Load(root)
	if err != nil {
		return Reply{}, err
	}
	if r.Action != "list" {
		if err := ValidateName(r.Name); err != nil {
			return Reply{}, err
		}
	}
	message := ""
	switch r.Action {
	case "list":
	case "add":
		if _, ok := c.Skills[r.Name]; !ok && len(c.Skills) >= MaxSkills {
			return Reply{}, fmt.Errorf("installation already holds %d skills", MaxSkills)
		}
		s, err := Read(Library(root), r.Name)
		if err != nil {
			return Reply{}, err
		}
		// Re-reading a registered skill refreshes its description without
		// disturbing where it is already switched on.
		s.Enabled = c.Skills[r.Name].Enabled
		c.Skills[r.Name] = s
		message = r.Name + " registered; enable it globally or for a project"
	case "remove":
		if _, ok := c.Skills[r.Name]; !ok {
			return Reply{}, fmt.Errorf("%s is not registered", r.Name)
		}
		delete(c.Skills, r.Name)
		for _, p := range c.Projects {
			delete(p, r.Name)
		}
		message = r.Name + " removed from the library; its directory is untouched"
	case "enable", "disable":
		if _, ok := c.Skills[r.Name]; !ok {
			return Reply{}, fmt.Errorf("%s is not registered", r.Name)
		}
		on := r.Action == "enable"
		if r.Project == "" {
			s := c.Skills[r.Name]
			s.Enabled = on
			c.Skills[r.Name] = s
			break
		}
		if c.Projects == nil {
			c.Projects = map[string]map[string]bool{}
		}
		if c.Projects[r.Project] == nil {
			c.Projects[r.Project] = map[string]bool{}
		}
		c.Projects[r.Project][r.Name] = on
	case "inherit":
		if r.Project == "" {
			return Reply{}, fmt.Errorf("inherit needs a project to restore")
		}
		delete(c.Projects[r.Project], r.Name)
		if len(c.Projects[r.Project]) == 0 {
			delete(c.Projects, r.Project)
		}
	default:
		return Reply{}, fmt.Errorf("unknown skill action: %s", r.Action)
	}
	if r.Action != "list" {
		if err := save(root, c); err != nil {
			return Reply{}, err
		}
	}
	reply := c.View(r.Project)
	reply.Message = message
	return reply, nil
}

func RuntimePath(root string) string { return filepath.Join(root, runtimeFile) }

// LoadRuntime reads what the manager last delivered to this project. A project
// that has never been told anything has no skills, not an error.
func LoadRuntime(root string) (filemap.Bundle, error) {
	b, err := os.ReadFile(RuntimePath(root))
	if os.IsNotExist(err) {
		return filemap.Bundle{}, nil
	}
	if err != nil {
		return filemap.Bundle{}, err
	}
	return filemap.Decode(b)
}

func SaveRuntime(root string, b filemap.Bundle) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return core.WriteJSON(RuntimePath(root), b)
}

// Delivered names the skills a bundle carries, read back from the destinations
// the manager wrote.
func Delivered(b filemap.Bundle) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range b.Files {
		rest, ok := strings.CutPrefix(filepath.ToSlash(f.Dst), "${AGENT_CONFIG_DIR}/skills/")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ApplyRuntime brings a session's skills directory to what the project was
// last told to have. cxz owns that directory: the library is how a skill gets
// there, so a name it did not deliver is one it delivered before and no longer
// does, and it goes. Dot-prefixed entries are left alone -- that is where the
// provider keeps its own skills.
func ApplyRuntime(root, agent, config, home, workspace string) error {
	b, err := LoadRuntime(root)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, name := range Delivered(b) {
		keep[name] = true
	}
	dir := filepath.Join(config, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || keep[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return filemap.ApplyBundle(b, agent, config, home, workspace)
}
