// Package memorylib manages provider-neutral, project-scoped Markdown memories.
package memorylib

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofrs/flock"
	"github.com/lesomnus/cxz/internal/core"
)

const MaxDocument = 256 * 1024
const MaxBundle = 16 * 1024 * 1024
const MaxDocuments = 256
const MaxMemories = 1000

type Client interface {
	Library(context.Context, string, Request) (Reply, error)
}
type Request struct {
	Cursor   string   `json:"cursor,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Action   string   `json:"action,omitempty"`
	IDs      []string `json:"ids,omitempty"`
	ID       string   `json:"id,omitempty"`
	Document string   `json:"document,omitempty"`
	Name     string   `json:"name,omitempty"`
	Content  string   `json:"content,omitempty"`
	Revision string   `json:"revision,omitempty"`
	Query    string   `json:"query,omitempty"`
	Trash    bool     `json:"trash,omitempty"`
}
type Memory struct {
	Format   int       `json:"format"`
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Session  string    `json:"session"`
	Agent    string    `json:"agent"`
	Created  time.Time `json:"created"`
	Snapshot bool      `json:"snapshot"`
	Parents  []string  `json:"parents,omitempty"`
	Size     int64     `json:"size"`
	Updated  time.Time `json:"updated"`
	Deleted  bool      `json:"deleted,omitempty"`
}
type Document struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Revision string    `json:"revision"`
	Deleted  bool      `json:"deleted,omitempty"`
}
type Reply struct {
	Changes       []Change   `json:"changes,omitempty"`
	Cursor        string     `json:"cursor,omitempty"`
	HasMore       bool       `json:"has_more,omitempty"`
	Baseline      bool       `json:"baseline,omitempty"`
	ResetRequired bool       `json:"reset_required,omitempty"`
	Location      string     `json:"location"`
	Memories      []Memory   `json:"memories,omitempty"`
	Memory        *Memory    `json:"memory,omitempty"`
	Documents     []Document `json:"documents,omitempty"`
	Content       string     `json:"content,omitempty"`
	Revision      string     `json:"revision,omitempty"`
	Message       string     `json:"message,omitempty"`
}
type Store struct {
	root    string
	Session core.Session
}

func key(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:12]) }
func New(root string, session core.Session) *Store {
	return &Store{filepath.Join(root, "memories", key(session.ProjectID)), session}
}
func (s *Store) OwnID() string { return "session-" + key(s.Session.ID) }

var validID = regexp.MustCompile(`^(session|saved)-[a-f0-9]{24}$`)
var validDoc = regexp.MustCompile(`^[\pL\pN][\pL\pN _.-]{0,120}\.md$`)

func deletedDoc(n string) bool {
	return len(n) > 32 && strings.HasPrefix(n, ".trash-") && regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(n[7:31]) && n[31] == '-' && validDoc.MatchString(n[32:])
}
func revision(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func read(r *os.Root, path string, limit int) ([]byte, error) {
	st, e := r.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	f, e := r.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if len(b) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	if e == nil && strings.HasSuffix(path, ".md") && (!utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0) {
		return nil, fmt.Errorf("memory must be UTF-8 Markdown")
	}
	return b, e
}
func write(r *os.Root, path string, b []byte) error {
	tmp := path + ".tmp-" + core.ID()
	f, e := r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer r.Remove(tmp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return r.Rename(tmp, path)
}
func entries(r *os.Root, path string, limit int) ([]os.DirEntry, error) {
	f, e := r.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	es, e := f.ReadDir(limit + 1)
	if errors.Is(e, io.EOF) {
		e = nil
	}
	if len(es) > limit {
		return nil, fmt.Errorf("too many entries in %s", path)
	}
	return es, e
}
func (s *Store) open() (*os.Root, error) {
	if s.Session.ID == "" || s.Session.ProjectID == "" {
		return nil, fmt.Errorf("project session required")
	}
	base := filepath.Dir(s.root)
	if e := os.MkdirAll(base, 0700); e != nil {
		return nil, e
	}
	st, e := os.Lstat(base)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("memory root must be a directory")
	}
	parent, e := os.OpenRoot(base)
	if e != nil {
		return nil, e
	}
	defer parent.Close()
	name := filepath.Base(s.root)
	if e = parent.Mkdir(name, 0700); e != nil && !os.IsExist(e) {
		return nil, e
	}
	st, e = parent.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("project memory must be a directory")
	}
	return parent.OpenRoot(name)
}
func load(r *os.Root, id string) (Memory, error) {
	var m Memory
	b, e := read(r, id+"/manifest.json", 65536)
	if e == nil {
		e = json.Unmarshal(b, &m)
	}
	m.ID = id
	if e == nil && m.Format != 1 {
		return m, fmt.Errorf("unsupported memory format %d", m.Format)
	}
	return m, e
}
func docs(r *os.Root, id string, trash bool) ([]Document, error) {
	es, e := entries(r, id, MaxDocuments*2+10)
	if e != nil {
		return nil, e
	}
	out := []Document{}
	for _, ent := range es {
		n := ent.Name()
		deleted := deletedDoc(n)
		if deleted != trash || (!deleted && !validDoc.MatchString(n)) {
			continue
		}
		if ent.Type()&os.ModeSymlink != 0 || ent.IsDir() {
			continue
		}
		b, e := read(r, id+"/"+n, MaxDocument)
		if e != nil {
			return nil, e
		}
		st, e := ent.Info()
		if e != nil {
			return nil, e
		}
		out = append(out, Document{n, int64(len(b)), st.ModTime(), revision(b), deleted})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (s *Store) ensure(r *os.Root) error {
	id := s.OwnID()
	if _, e := load(r, id); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := r.MkdirAll(id, 0700); e != nil {
		return e
	}
	name := s.Session.Title
	if strings.TrimSpace(name) == "" {
		name = "Session " + s.Session.ID
	}
	m := Memory{Format: 1, ID: id, Name: name, Session: s.Session.ID, Agent: s.Session.Kind, Created: time.Now().UTC()}
	b, _ := json.MarshalIndent(m, "", "  ")
	return write(r, id+"/manifest.json", b)
}
func (s *Store) Do(ctx context.Context, q Request) (Reply, error) {
	if q.Action == "import" {
		return s.ImportNative(ctx, filepath.Dir(filepath.Dir(s.root)))
	}
	out := Reply{Location: s.root}
	r, e := s.open()
	if e != nil {
		return out, e
	}
	defer r.Close()
	lock := flock.New(filepath.Join(s.root, ".lock"))
	ok, e := lock.TryLockContext(ctx, 25*time.Millisecond)
	if e != nil {
		return out, e
	}
	if !ok {
		return out, ctx.Err()
	}
	defer lock.Unlock()
	if q.Action == "init" || q.Action == "update" || q.Action == "fork" {
		if e = s.ensure(r); e != nil {
			return out, e
		}
	}
	if q.Action == "" {
		q.Action = "list"
	}
	if q.ID == "" {
		q.ID = s.OwnID()
	}
	if !validID.MatchString(q.ID) {
		return out, fmt.Errorf("invalid memory ID")
	}
	dir := q.ID
	if q.Trash {
		dir = "trash/" + dir
	}
	switch q.Action {
	case "changes":
		return s.changes(ctx, r, q)
	case "init":
		return out, nil
	case "list", "search":
		base := "."
		if q.Trash {
			base = "trash"
		}
		es, e := entries(r, base, MaxMemories+5)
		if os.IsNotExist(e) {
			return out, nil
		}
		if e != nil {
			return out, e
		}
		for _, ent := range es {
			if !validID.MatchString(ent.Name()) || !ent.IsDir() {
				continue
			}
			id := ent.Name()
			rel := id
			if q.Trash {
				rel = "trash/" + id
			}
			m, e := load(r, rel)
			if e != nil {
				return out, e
			}
			m.ID = id
			m.Deleted = q.Trash
			m.Updated = m.Created
			ds, e := docs(r, rel, false)
			if e != nil {
				return out, e
			}
			match := q.Query == "" || strings.Contains(strings.ToLower(m.Name), strings.ToLower(q.Query))
			for _, d := range ds {
				m.Size += d.Size
				if d.Modified.After(m.Updated) {
					m.Updated = d.Modified
				}
				if !match {
					b, e := read(r, rel+"/"+d.Name, MaxDocument)
					if e != nil {
						return out, e
					}
					match = strings.Contains(strings.ToLower(d.Name+" "+string(b)), strings.ToLower(q.Query))
				}
			}
			if match {
				out.Memories = append(out.Memories, m)
			}
		}
		sort.Slice(out.Memories, func(i, j int) bool { return out.Memories[i].Updated.After(out.Memories[j].Updated) })
		return out, nil
	case "read":
		m, e := load(r, dir)
		if e != nil {
			return out, e
		}
		m.ID = q.ID
		m.Deleted = q.Trash
		out.Memory = &m
		out.Location = filepath.Join(s.root, dir)
		if q.Document == "" {
			out.Documents, e = docs(r, dir, false)
			return out, e
		}
		if !validDoc.MatchString(q.Document) {
			return out, fmt.Errorf("invalid Markdown filename")
		}
		b, e := read(r, dir+"/"+q.Document, MaxDocument)
		if e != nil {
			return out, e
		}
		out.Content = string(b)
		out.Revision = revision(b)
		return out, nil
	case "update":
		if q.ID != s.OwnID() || q.Trash {
			return out, fmt.Errorf("only this session's working memory can be updated")
		}
		if !validDoc.MatchString(q.Document) || !utf8.ValidString(q.Content) || strings.ContainsRune(q.Content, 0) || len(q.Content) > MaxDocument {
			return out, fmt.Errorf("invalid Markdown document (maximum 256 KiB)")
		}
		ds, e := docs(r, dir, false)
		if e != nil {
			return out, e
		}
		total := len(q.Content)
		found := false
		for _, d := range ds {
			if d.Name == q.Document {
				found = true
			} else {
				total += int(d.Size)
			}
		}
		if total > MaxBundle || (!found && len(ds) >= MaxDocuments) {
			return out, fmt.Errorf("memory capacity exceeded")
		}
		old, e := read(r, dir+"/"+q.Document, MaxDocument)
		if e != nil && !os.IsNotExist(e) {
			return out, e
		}
		if (e == nil && q.Revision != revision(old)) || (os.IsNotExist(e) && q.Revision != "") {
			return out, fmt.Errorf("memory changed; read latest revision before updating")
		}
		e = write(r, dir+"/"+q.Document, []byte(q.Content))
		out.Revision = revision([]byte(q.Content))
		return out, e
	case "snapshot", "fork", "merge":
		if q.Trash {
			return out, fmt.Errorf("restore memory before using it")
		}
		ids := []string{q.ID}
		if q.Action == "merge" {
			ids = q.IDs
			if len(ids) < 2 || len(ids) > 8 {
				return out, fmt.Errorf("select 2 to 8 memories")
			}
		}
		return s.copy(r, ids, q.Name, q.Action == "fork")
	case "rename":
		if strings.TrimSpace(q.Name) == "" || len(q.Name) > 256 {
			return out, fmt.Errorf("name required (maximum 256 bytes)")
		}
		m, e := load(r, dir)
		if e != nil {
			return out, e
		}
		m.Name = q.Name
		b, _ := json.MarshalIndent(m, "", "  ")
		return out, write(r, dir+"/manifest.json", b)
	case "delete":
		if q.Trash {
			return out, fmt.Errorf("already deleted")
		}
		if q.Document != "" {
			if !validDoc.MatchString(q.Document) {
				return out, fmt.Errorf("invalid Markdown filename")
			}
			b, e := read(r, dir+"/"+q.Document, MaxDocument)
			if e != nil {
				return out, e
			}
			if q.Revision != revision(b) {
				return out, fmt.Errorf("memory changed; reload before deleting")
			}
			e = r.Rename(dir+"/"+q.Document, dir+"/.trash-"+core.ID()+"-"+q.Document)
			return out, e
		}
		if e = r.MkdirAll("trash", 0700); e != nil {
			return out, e
		}
		if _, e = r.Stat("trash/" + q.ID); e == nil {
			return out, fmt.Errorf("a deleted memory with this ID already exists; restore it first")
		}
		return out, r.Rename(dir, "trash/"+q.ID)
	case "restore":
		if q.Document != "" {
			if !deletedDoc(q.Document) {
				return out, fmt.Errorf("invalid deleted document")
			}
			target := dir + "/" + q.Document[32:]
			if _, e = r.Stat(target); e == nil {
				return out, fmt.Errorf("document already exists")
			}
			return out, r.Rename(dir+"/"+q.Document, target)
		}
		target := q.ID
		if _, e = r.Stat(target); e == nil {
			// A live agent may have recreated its working memory after deletion.
			// Restore the older contents as an independent saved copy instead of overwriting.
			target = "saved-" + core.ID()
			m, e := load(r, "trash/"+q.ID)
			if e != nil {
				return out, e
			}
			m.ID = target
			m.Snapshot = true
			m.Parents = append(m.Parents, q.ID)
			data, _ := json.MarshalIndent(m, "", "  ")
			if e = write(r, "trash/"+q.ID+"/manifest.json", data); e != nil {
				return out, e
			}
		}
		return out, r.Rename("trash/"+q.ID, target)
	case "purge":
		if q.Document != "" {
			if !deletedDoc(q.Document) {
				return out, fmt.Errorf("only deleted documents can be permanently removed")
			}
			return out, r.Remove(dir + "/" + q.Document)
		}
		if !q.Trash {
			return out, fmt.Errorf("move memory to trash first")
		}
		return out, r.RemoveAll(dir)
	case "deleted_documents":
		out.Documents, e = docs(r, dir, true)
		return out, e
	default:
		return out, fmt.Errorf("unknown memory operation")
	}
}
func (s *Store) copy(r *os.Root, ids []string, name string, fork bool) (Reply, error) {
	out := Reply{Location: s.root}
	target := "saved-" + core.ID()
	if fork {
		target = s.OwnID()
		ds, e := docs(r, target, false)
		if e != nil {
			return out, e
		}
		all, e := entries(r, target, MaxDocuments*2+10)
		if e != nil {
			return out, e
		}
		if len(ds) != 0 || len(all) != 1 || all[0].Name() != "manifest.json" {
			return out, fmt.Errorf("target session already has memory")
		}
	}
	if !fork {
		es, e := entries(r, ".", MaxMemories+10)
		if e != nil {
			return out, e
		}
		count := 0
		for _, ent := range es {
			if ent.IsDir() && validID.MatchString(ent.Name()) {
				count++
			}
		}
		if count >= MaxMemories {
			return out, fmt.Errorf("memory collection limit reached; remove unused memories")
		}
	}
	stage := ".stage-" + core.ID()
	if e := r.Mkdir(stage, 0700); e != nil {
		return out, e
	}
	defer r.RemoveAll(stage)
	count, total := 0, 0
	for index, id := range ids {
		if !validID.MatchString(id) {
			return out, fmt.Errorf("invalid memory ID")
		}
		if _, e := load(r, id); e != nil {
			return out, e
		}
		ds, e := docs(r, id, false)
		if e != nil {
			return out, e
		}
		for _, d := range ds {
			b, e := read(r, id+"/"+d.Name, MaxDocument)
			if e != nil {
				return out, e
			}
			count++
			total += len(b)
			if count > MaxDocuments || total > MaxBundle {
				return out, fmt.Errorf("memory capacity exceeded")
			}
			n := d.Name
			if len(ids) > 1 {
				n = fmt.Sprintf("%d-%s", index+1, n)
				if !validDoc.MatchString(n) {
					runes := []rune(strings.TrimSuffix(n, ".md"))
					n = string(runes[:min(80, len(runes))]) + "-" + key(n)[:8] + ".md"
				}
			}
			if e = write(r, stage+"/"+n, b); e != nil {
				return out, e
			}
		}
	}
	if name == "" {
		name = "Memory " + time.Now().Format("2006-01-02 15:04")
	}
	if len(name) > 256 {
		return out, fmt.Errorf("name too long")
	}
	m := Memory{Format: 1, ID: target, Name: name, Session: s.Session.ID, Agent: s.Session.Kind, Created: time.Now().UTC(), Snapshot: !fork, Parents: ids}
	if !fork && len(ids) == 1 {
		source, e := load(r, ids[0])
		if e != nil {
			return out, e
		}
		m.Session = source.Session
		m.Agent = source.Agent
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if e := write(r, stage+"/manifest.json", b); e != nil {
		return out, e
	}
	if fork {
		backup := ".empty-" + core.ID()
		if e := r.Rename(target, backup); e != nil {
			return out, e
		}
		if e := r.Rename(stage, target); e != nil {
			_ = r.Rename(backup, target)
			return out, e
		}
		_ = r.RemoveAll(backup)
	} else {
		if e := r.Rename(stage, target); e != nil {
			return out, e
		}
	}
	out.Memory = &m
	out.Location = filepath.Join(s.root, target)
	return out, nil
}
