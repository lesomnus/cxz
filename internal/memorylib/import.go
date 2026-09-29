package memorylib

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"io/fs"
	"os"
	"strings"
)

// ImportNative copies Markdown memory/instructions only, never transcripts or credentials.
func (s *Store) ImportNative(ctx context.Context, root string) (Reply, error) {
	out := Reply{}
	if s.Session.CreateID == "" {
		return out, fmt.Errorf("session profile unavailable")
	}
	if e := accounts.Validate(s.Session.Account, s.Session.Kind); e != nil {
		return out, e
	}
	r, e := os.OpenRoot(accounts.Config(accounts.SessionRoot(root, s.Session.CreateID), s.Session.Account))
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	defer r.Close()
	count, total, visited := 0, 0, 0
	e = fs.WalkDir(r.FS(), ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 10000 {
			return fmt.Errorf("native memory scan limit reached")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if path == "." || path == "projects" || path == "memory" || path == "memories" || strings.HasPrefix(path, "memory/") || strings.HasPrefix(path, "memories/") {
				return nil
			}
			parts := strings.Split(path, "/")
			if parts[0] == "projects" && (len(parts) == 2 || len(parts) >= 3 && parts[2] == "memory") {
				return nil
			}
			return fs.SkipDir
		}
		allowed := path == "CLAUDE.md" && s.Session.Kind == "claude" || path == "AGENTS.md" && s.Session.Kind == "codex" || strings.HasPrefix(path, "memory/") || strings.HasPrefix(path, "memories/")
		parts := strings.Split(path, "/")
		if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "memory" {
			allowed = true
		}
		if !allowed || !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, e := read(r, path, MaxDocument)
		if e != nil {
			return e
		}
		count++
		total += len(b)
		if count > MaxDocuments || total > MaxBundle {
			return fmt.Errorf("native memory capacity exceeded")
		}
		// Content-addressed imports preserve both previous imports and external edits.
		name := "import-" + key(path+"\x00"+string(b)) + ".md"
		old, e := s.Do(ctx, Request{Action: "read", Document: name})
		if e == nil {
			return nil
		}
		if !os.IsNotExist(e) {
			return e
		}
		_, e = s.Do(ctx, Request{Action: "update", Document: name, Revision: old.Revision, Content: "<!-- Imported from " + s.Session.Kind + ": " + strings.ReplaceAll(path, "-->", "") + " -->\n\n" + string(b)})
		return e
	})
	out.Message = fmt.Sprintf("Imported %d native Markdown files; existing notes preserved", count)
	return out, e
}
