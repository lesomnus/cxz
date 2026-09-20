package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/filemap"
	"google.golang.org/grpc"
)

type filesClient struct {
	api.SessionsClient
	bundle []byte
	err    error
}

func (c *filesClient) FileMappings(_ context.Context, r *api.FileMappingsInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.bundle = r.Bundle
	return &api.Receipt{}, c.err
}
func TestFileMappingsPersistForFutureProjects(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	client := &filesClient{}
	if err = m.syncFileMappings(t.Context(), client); err != nil || client.bundle != nil {
		t.Fatal("unconfigured installation contacted runtime", err)
	}
	bundle := filemap.Bundle{Files: []filemap.File{{Dst: "${AGENT_CONFIG_DIR}/CLAUDE.md", Content: []byte("saved instructions")}}}
	if _, err = m.FileMappings(t.Context(), bundle); err != nil {
		t.Fatal(err)
	}
	// A replacement manager can serve projects which were stopped during sync.
	m, err = New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.syncFileMappings(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	received, err := filemap.Decode(client.bundle)
	if err != nil || len(received.Files) != 1 || string(received.Files[0].Content) != "saved instructions" {
		t.Fatal("snapshot not retained", err)
	}
	client.err = errors.New("old runtime")
	if err = m.syncFileMappings(t.Context(), client); err == nil {
		t.Fatal("ignored failed delivery before launch")
	}
	client.err = nil
	if _, err = m.FileMappings(t.Context(), filemap.Bundle{}); err != nil {
		t.Fatal(err)
	}
	if err = m.syncFileMappings(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	received, err = filemap.Decode(client.bundle)
	if err != nil || len(received.Files) != 0 {
		t.Fatal("removed mappings not propagated", err)
	}
}
