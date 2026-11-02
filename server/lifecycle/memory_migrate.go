package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/memorymigration"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/resource"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"os"
	"path/filepath"
	"time"
)

type memoryRoots interface{ MemoryRoot() (string, bool) }

// MigrateMemories is resumable. It never removes source files and commits an
// import marker together with each document/version, including deleted copies.
func (s Layer) MigrateMemories(ctx context.Context) error {
	roots, ok := s.shared.runtime.(memoryRoots)
	if !ok {
		return nil
	}
	root, local := roots.MemoryRoot()
	if !local || root == "" {
		return nil
	}
	if err := s.ensureSnapshot(ctx); err != nil {
		return err
	}
	var runtime struct{ ProjectID, LegacyProjectID string }
	b, err := os.ReadFile(filepath.Join(filepath.Dir(root), "runtime.json"))
	single := err == nil && json.Unmarshal(b, &runtime) == nil && runtime.ProjectID != ""
	projects, err := s.Next().Project().List(ctx, &resource.ProjectListRequest{})
	if err != nil {
		return err
	}
	for _, project := range projects.GetItems() {
		id := projectid.String(project.GetId())
		if single && !projectid.Same(id, runtime.ProjectID) {
			continue
		}
		key := sha256.Sum256([]byte(id))
		expected := hex.EncodeToString(key[:12])
		err = memorymigration.Scan(ctx, root, func(folder, session string) bool {
			if single || folder == expected {
				return true
			}
			v, e := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(session), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
			return e == nil && projectid.String(v.GetProject().GetId()) == id
		}, func(d memorymigration.Document) error { return s.importMemory(ctx, project, d) })
		if err != nil {
			return fmt.Errorf("migrate project %s memories: %w", id, err)
		}
	}
	return nil
}
func (s Layer) importMemory(ctx context.Context, project *resource.Project, d memorymigration.Document) error {
	source := projectid.String(project.GetId()) + ":" + d.Source
	var oldID, oldHash string
	err := s.shared.db.QueryRowContext(ctx, `SELECT memory_id,content_hash FROM memory_migration WHERE source=?`, source).Scan(&oldID, &oldHash)
	if err == nil && oldHash == d.Hash {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	owner := projectid.String(resourceid.Bytes(8, d.Session))
	v, e := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(d.Session), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if e == nil {
		owner = projectid.String(v.GetId())
	}
	labels := map[string]string{"legacy.collection": d.Collection, "legacy.collection-name": d.CollectionName, "legacy.filename": d.Name, "legacy.agent": d.Agent}
	if d.Snapshot {
		labels["legacy.snapshot"] = "true"
	}
	id := resourceid.Bytes(18, "memory:"+source)
	clientID := "legacy:" + source
	existing, e := s.Next().Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: resource.MemoryById(id), Select: memoryAll()}.Build())
	if e != nil && status.Code(e) != codes.NotFound {
		return e
	}
	if e == nil {
		hash := sha256.Sum256([]byte(existing.GetContent()))
		if oldHash == "" || hex.EncodeToString(hash[:]) != oldHash {
			// A still-running old binary edited its file after a user edited the new
			// resource. Preserve both instead of overwriting the user's new version.
			id = resourceid.Bytes(18, "memory-conflict:"+source+":"+d.Hash)
			clientID += "/" + d.Hash
			labels["legacy.conflict"] = "true"
			existing = nil
		}
	}
	driver, tx, err := dialect.BeginTx(ctx, entsql.OpenDB(dialect.SQLite, s.shared.db))
	if err != nil {
		return err
	}
	defer tx.Rollback()
	store, err := enttx.Rebind(s.Next(), driver)
	if err != nil {
		return err
	}
	var m *resource.Memory
	if existing == nil {
		created := d.Created
		if created.IsZero() {
			created = d.Modified
		}
		request := resource.MemoryAddRequest_builder{Id: id, Alias: "import-" + projectid.String(id), Name: d.Name, Content: d.Content, Project: projectRef(projectid.String(project.GetId())), OwnerSession: owner, Revision: 1, Labels: labels, ClientId: clientID, DateCreated: timestamppb.New(created)}.Build()
		if d.Deleted {
			request.SetDateDeleted(timestamppb.New(time.Now().Add(30 * 24 * time.Hour)))
		}
		m, err = store.Memory().Add(ctx, request)
	} else {
		m, err = store.Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: existing.Ref(), DateUpdated: existing.GetDateUpdated(), Content: ptr(d.Content), Revision: ptr(existing.GetRevision() + 1)}.Build())
	}
	if err != nil {
		return err
	}
	err = driver.Exec(ctx, `INSERT INTO memory_migration(source,memory_id,content_hash,date_created) VALUES(?,?,?,?) ON CONFLICT(source) DO UPDATE SET memory_id=excluded.memory_id,content_hash=excluded.content_hash`, []any{source, projectid.String(m.GetId()), d.Hash, time.Now().UTC()}, nil)
	if err != nil {
		return err
	}
	return tx.Commit()
}
