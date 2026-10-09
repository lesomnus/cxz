package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/protobuf/proto"
)

func renderFixture(t *testing.T) (*Manager, *Project) {
	t.Helper()
	root := t.TempDir()
	m := &Manager{Root: root, Owner: strings.Repeat("o", 24)}
	p := &Project{ID: "abc", Name: "demo", Workspace: t.TempDir()}
	dir := filepath.Join(root, "projects", p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(p.Workspace, "docker-compose.yaml")
	if err := os.WriteFile(own, []byte("services:\n  dev:\n    image: alpine\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"compose.user.json": `{"services":{}}`,
		"compose.json":      `{"name":"cxz-x-abc","services":{"dev":{}}}`,
		"devcontainer.json": `{"service":"dev","dockerComposeFile":["` + own + `","` + filepath.Join(dir, "compose.user.json") + `","` + filepath.Join(dir, "compose.json") + `"],"containerEnv":{"CXZ_PROJECT_ID":"abc"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return m, p
}

// The order is the whole answer: a later file overrides an earlier one, so a
// report that loses the order reports nothing worth reading.
func TestRenderReportsComposeFilesInMergeOrder(t *testing.T) {
	m, p := renderFixture(t)
	reply, err := m.renderProjectDevcontainer(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range reply.Files {
		names = append(names, f.Name)
		if len(f.Data) == 0 || f.Role == "" || f.Source == "" {
			t.Fatal("incomplete entry", f.Name, f.Role, f.Source)
		}
	}
	want := []string{"devcontainer.json", "compose/01-docker-compose.yaml", "compose/02-compose.user.json", "compose/03-compose.json"}
	for i, name := range want {
		if i >= len(names) || names[i] != name {
			t.Fatal("files out of merge order:", names)
		}
	}
	// Each cxz-owned file says which one it is, since that is exactly what a
	// reader cannot tell from a name under a state directory.
	roles := map[string]string{"compose/02-compose.user.json": "shared", "compose/03-compose.json": "cxz's own"}
	for _, f := range reply.Files {
		if want, ok := roles[f.Name]; ok && !strings.Contains(f.Role, want) {
			t.Fatal(f.Name, "role does not name it:", f.Role)
		}
	}
	// The merged result needs the Compose CLI; without one the files still
	// report, with the reason attached rather than an error in its place.
	if len(reply.Files) == len(want) && reply.Note == "" {
		t.Fatal("missing merged result and no note explaining it")
	}
	if reply.Project != "abc" || reply.Workspace != p.Workspace {
		t.Fatal("reply does not identify the project", reply.Project, reply.Workspace)
	}
}

// An unprovisioned project has no answer to give, and inventing one is the
// failure this command exists to avoid.
func TestRenderRefusesAnUnprovisionedProject(t *testing.T) {
	m := &Manager{Root: t.TempDir(), Owner: strings.Repeat("o", 24)}
	_, err := m.renderProjectDevcontainer(context.Background(), &Project{ID: "none", Workspace: "/w"})
	if err == nil || !strings.Contains(err.Error(), "cxz up") {
		t.Fatal("unprovisioned project not reported as such:", err)
	}
}

// The project's own file is included so cxz's additions can be read as a
// difference, and it is labelled as a fresh discovery because nothing on disk
// records which file provisioning read.
func TestRenderIncludesTheProjectsOwnConfiguration(t *testing.T) {
	m, p := renderFixture(t)
	own := filepath.Join(p.Workspace, ".devcontainer.json")
	if err := os.WriteFile(own, []byte(`{"image":"alpine"}`), 0600); err != nil {
		t.Fatal(err)
	}
	reply, err := m.renderProjectDevcontainer(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range reply.Files {
		if f.Source == own {
			if !strings.Contains(f.Role, "now") {
				t.Fatal("source file claims to be a record of provisioning:", f.Role)
			}
			return
		}
	}
	t.Fatal("the project's own configuration was left out")
}

// Image-based projects have no Compose to merge, and saying so beats an empty
// directory the reader has to interpret.
func TestRenderExplainsAnImageDevcontainer(t *testing.T) {
	m, p := renderFixture(t)
	dir := filepath.Join(m.Root, "projects", p.ID)
	if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte(`{"image":"alpine"}`), 0600); err != nil {
		t.Fatal(err)
	}
	reply, err := m.renderProjectDevcontainer(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Files) != 1 || !strings.Contains(reply.Note, "Compose") {
		t.Fatal("image devcontainer not explained", len(reply.Files), reply.Note)
	}
}

// The reply crosses a connection, so the file bytes have to survive that trip
// intact: a configuration that arrives truncated is read as the one in effect.
func TestRenderReplySurvivesTheWire(t *testing.T) {
	m, p := renderFixture(t)
	reply, err := m.renderProjectDevcontainer(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &api.RenderDevcontainerReply{}
	if err = proto.Unmarshal(b, decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Files) != len(reply.Files) {
		t.Fatal("files lost in transit", len(decoded.Files))
	}
	for i, f := range decoded.Files {
		if string(f.Data) != string(reply.Files[i].Data) || f.Name != reply.Files[i].Name {
			t.Fatal("file changed in transit:", f.Name)
		}
	}
	if !strings.Contains(string(decoded.Files[0].Data), "CXZ_PROJECT_ID") {
		t.Fatal("the configuration cxz applies is not what was returned")
	}
}

// The question gets asked from wherever you are in a repository, which is often
// a subdirectory -- .devcontainer most of all. Only a registered workspace
// matches, so walking up finds the project without widening the answer.
func TestRenderResolvesFromASubdirectory(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db, Root: t.TempDir(), Owner: strings.Repeat("o", 24)}
	workspace := t.TempDir()
	p := &Project{ID: "abc", Name: "demo", Workspace: workspace}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO projects(id,data) VALUES(?,?)", p.ID, b); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(workspace, ".devcontainer", "deep")
	if err = os.MkdirAll(inside, 0700); err != nil {
		t.Fatal(err)
	}
	for _, handle := range []string{workspace, inside, "abc", "demo"} {
		got, err := m.resolveNearest(t.Context(), handle)
		if err != nil || got.ID != p.ID {
			t.Fatal("did not resolve", handle, err)
		}
	}
	// A path outside every workspace is still an error, and it names what was
	// asked for rather than the root it gave up at.
	_, err = m.resolveNearest(t.Context(), filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil || !strings.Contains(err.Error(), "elsewhere") {
		t.Fatal("unregistered path not reported as asked:", err)
	}
}
