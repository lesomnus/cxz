// Package memorymigration reads the retired document-collection format. It is
// an import reader only: all live reads and writes use MemoryService.
package memorymigration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type Document struct {
	Source, Collection, CollectionName, Name, Session, Agent, Content, Hash string
	Snapshot, Deleted                                                       bool
	Created, Modified                                                       time.Time
}
type manifest struct {
	Name, Session, Agent string
	Snapshot             bool
	Created              time.Time
	Format               int
}

// Scan refuses partial/corrupt imports. The retained source remains untouched.
func Scan(ctx context.Context, root string, accept func(string, string) bool, consume func(Document) error) error {
	base := filepath.Join(root, "memories")
	projects, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, project := range projects {
		if !project.IsDir() || len(project.Name()) != 24 {
			continue
		}
		dir := filepath.Join(base, project.Name())
		lock := flock.New(filepath.Join(dir, ".lock"))
		ok, err := lock.TryLockContext(ctx, 25*time.Millisecond)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("legacy memory is busy")
		}
		err = scanProject(ctx, dir, project.Name(), accept, consume)
		unlock := lock.Unlock()
		_ = lock.Close()
		if err != nil {
			return err
		}
		if unlock != nil {
			return unlock
		}
	}
	return nil
}
func scanProject(ctx context.Context, dir, project string, accept func(string, string) bool, consume func(Document) error) error {
	for _, trash := range []bool{false, true} {
		path := dir
		if trash {
			path = filepath.Join(dir, "trash")
		}
		entries, err := os.ReadDir(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || (!strings.HasPrefix(entry.Name(), "session-") && !strings.HasPrefix(entry.Name(), "saved-")) {
				continue
			}
			bundle := filepath.Join(path, entry.Name())
			raw, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
			if err != nil {
				return err
			}
			var m manifest
			if err = json.Unmarshal(raw, &m); err != nil {
				return err
			}
			if m.Format != 1 {
				return fmt.Errorf("unsupported legacy memory format %d", m.Format)
			}
			if accept != nil && !accept(project, m.Session) {
				continue
			}
			docs, err := os.ReadDir(bundle)
			if err != nil {
				return err
			}
			for _, file := range docs {
				if err := ctx.Err(); err != nil {
					return err
				}
				name := file.Name()
				deleted := trash
				if strings.HasPrefix(name, ".trash-") && len(name) > 32 {
					name = name[32:]
					deleted = true
				}
				if file.IsDir() || file.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(name, ".md") {
					continue
				}
				info, err := file.Info()
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() || info.Size() > 256*1024 {
					return fmt.Errorf("invalid legacy document %s", file.Name())
				}
				data, err := os.ReadFile(filepath.Join(bundle, file.Name()))
				if err != nil {
					return err
				}
				if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
					return fmt.Errorf("legacy document is not UTF-8 Markdown")
				}
				hash := sha256.Sum256(data)
				source := entry.Name() + "/" + file.Name()
				if trash {
					source = "trash/" + source
				}
				if err = consume(Document{Source: source, Collection: entry.Name(), CollectionName: m.Name, Name: name, Session: m.Session, Agent: m.Agent, Snapshot: m.Snapshot, Deleted: deleted, Created: m.Created, Modified: info.ModTime(), Content: string(data), Hash: hex.EncodeToString(hash[:])}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
