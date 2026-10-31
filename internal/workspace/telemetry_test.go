package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"encoding/hex"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/telemetrystore"
)

func TestTelemetryRegistryAuxiliaryAttributionAndParentRevocation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "telemetry", "auth"), 0700); err != nil {
		t.Fatal(err)
	}
	p := &Project{ID: "00112233445566778899aabbccddeeff", Token: "project-capability", Sessions: []*api.Session{{Id: "parent", Account: "main", Agent: "codex"}}}
	m := &Manager{Root: dir, telemetry: telemetrystore.New("http://unused", "unused"), telemetryAux: map[string]map[string]telemetryauth.Session{p.ID: {"job": {Account: "helper", Agent: "claude", Scope: "auxiliary", Parent: "parent"}}}}
	if err := m.writeTelemetryRegistry(p); err != nil {
		t.Fatal(err)
	}
	i := telemetryauth.Identity{Project: p.ID, Session: "job", Parent: "parent", Account: "helper", Agent: "claude", Run: "run", Scope: "auxiliary", Source: "supervisor", UsageSource: "supervisor"}
	token, err := telemetryauth.Sign(p.Token, i)
	if err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(dir, "telemetry", "auth")
	verified, err := telemetryauth.Verify(auth, token)
	if err != nil {
		t.Fatal(err)
	}
	attrs := verified.Attributes()
	if attrs["cxz.session.id"] != hex.EncodeToString(resourceid.Bytes(8, "parent")) || attrs["cxz.account.id"] != hex.EncodeToString(resourceid.Bytes(9, "helper")) {
		t.Fatal("auxiliary account or parent attribution lost", attrs)
	}
	m.removedSessions = map[string]bool{"parent": true}
	if err = m.writeTelemetryRegistry(p); err != nil {
		t.Fatal(err)
	}
	if _, err = telemetryauth.Verify(auth, token); err == nil {
		t.Fatal("erased parent did not revoke auxiliary ingestion")
	}
}
