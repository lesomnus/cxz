package accounts

import (
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuxiliaryGrantCannotCrossProjectScope(t *testing.T) {
	root := t.TempDir()
	g := Grant{Socket: BrokerSocket, Capability: strings.Repeat("a", 64), Scope: "auxiliary", Account: "work", Binding: AuxiliaryBindingID("work"), AccountID: "fake"}
	if e := InstallGrant(root, g); e != nil {
		t.Fatal(e)
	}
	if _, e := FetchToken(context.Background(), root, "work", "", g.Binding, "", false); e == nil || !strings.Contains(e.Error(), "mismatch") {
		t.Fatal("auxiliary grant accepted as project", e)
	}
	g.Project = "project"
	if e := InstallGrant(root, g); e == nil {
		t.Fatal("auxiliary grant accepted project")
	}
	g.Scope = ""
	g.Binding = BindingID("project", "work", BrokeredAccessToken)
	if e := InstallGrant(root, g); e != nil {
		t.Fatal(e)
	}
	if _, e := FetchAuxiliaryToken(context.Background(), root, "work", "", false); e == nil || !strings.Contains(e.Error(), "mismatch") {
		t.Fatal("project grant accepted as auxiliary", e)
	}
}

func TestRevokeAuxiliaryDoesNotRemoveProjectGrant(t *testing.T) {
	root := t.TempDir()
	g := Grant{Socket: BrokerSocket, Capability: strings.Repeat("b", 64), Scope: "auxiliary", Account: "work", Binding: AuxiliaryBindingID("work"), AccountID: "fake"}
	path := filepath.Join(root, "central", "bindings", g.Binding+".json")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Dir(capabilityPath(root, g.Capability)), 0700); e != nil {
		t.Fatal(e)
	}
	if e := core.WriteJSON(path, g); e != nil {
		t.Fatal(e)
	}
	if e := core.WriteJSON(capabilityPath(root, g.Capability), g); e != nil {
		t.Fatal(e)
	}
	project := capabilityPath(root, strings.Repeat("c", 64))
	if e := os.WriteFile(project, []byte("project fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := RevokeAuxiliaryGrant(root, "work"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if _, e := os.Stat(capabilityPath(root, g.Capability)); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if _, e := os.Stat(project); e != nil {
		t.Fatal("removed project grant", e)
	}
}
