package telemetryauth

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceid"
)

func TestCapabilityIsolationAndRevocation(t *testing.T) {
	i := Identity{Project: "00112233-4455-6677-8899-aabbccddeeff", Session: "runtime", Account: "work", Agent: "codex", Run: "run", Source: "supervisor", UsageSource: "supervisor", Scope: "conversation"}
	key, _ := ProjectKey(i.Project)
	dir := t.TempDir()
	file := filepath.Join(dir, key+".json")
	registry := Registry{Secret: "private-project-capability", Sessions: map[string]Session{i.Session: {Account: i.Account, Agent: i.Agent}}}
	if err := core.WriteJSON(file, registry); err != nil {
		t.Fatal(err)
	}
	token, err := Sign(registry.Secret, i)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(dir, token)
	if err != nil || got != i {
		t.Fatal(got, err)
	}
	attrs := got.Attributes()
	if attrs["cxz.project.id"] != key || attrs["cxz.session.id"] != hex.EncodeToString(resourceid.Bytes(8, i.Session)) || attrs["cxz.account.id"] != hex.EncodeToString(resourceid.Bytes(9, i.Account)) {
		t.Fatal(attrs)
	}
	if _, err = VerifyProject("ffffffffffffffffffffffffffffffff", registry.Secret, token); err == nil {
		t.Fatal("cross-project relay accepted")
	}
	for _, bad := range []string{token + "x", strings.Repeat("a", 4097), "../other.file", "", "Bearer " + token} {
		if _, err = Verify(dir, bad); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	i.Account = "another"
	bad, _ := Sign(registry.Secret, i)
	if _, err = Verify(dir, bad); err == nil {
		t.Fatal("wrong account accepted")
	}
	delete(registry.Sessions, "runtime")
	if err = core.WriteJSON(file, registry); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(dir, token); err == nil {
		t.Fatal("revoked session accepted")
	}
	if err = os.WriteFile(file, make([]byte, (4<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(dir, token); err == nil {
		t.Fatal("oversized registry accepted")
	}
}
