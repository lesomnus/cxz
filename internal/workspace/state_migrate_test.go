package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// migrationFake answers the two questions the migration asks docker: did the
// copy run, and do the two trees hold the same files.
type migrationFake struct {
	calls []string
	from  string
	to    string
	fail  string
}

func (f *migrationFake) run(_ context.Context, args ...string) ([]byte, error) {
	line := strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.fail != "" && strings.Contains(line, f.fail) {
		return nil, errContext
	}
	switch {
	case strings.Contains(line, "cd /from "):
		return []byte(f.from), nil
	case strings.Contains(line, "cd /to/"):
		return []byte(f.to), nil
	}
	return nil, nil
}

var errContext = context.Canceled

func migrating(t *testing.T) (*Manager, *Project) {
	t.Helper()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db, Root: t.TempDir(), Owner: strings.Repeat("o", 24), Image: "manager:test", StateVolume: "cxz-owner-state"}
	return m, &Project{ID: strings.Repeat("a", 24), Volume: "cxz-owner-aaa-state"}
}

// A copy that matches lets the volume go, and the project stops referring to it.
func TestMigrationMovesStateWhenTheCopyMatches(t *testing.T) {
	m, p := migrating(t)
	f := &migrationFake{from: "10 ./cxz.db\n20 ./sessions/x/events.jsonl\n", to: "10 ./cxz.db\n20 ./sessions/x/events.jsonl\n"}
	if err := m.migrateProjectStateWith(context.Background(), p, f.run); err != nil {
		t.Fatal(err)
	}
	if p.Volume != "" {
		t.Fatal("the project still refers to its volume:", p.Volume)
	}
	var copied, removed bool
	for _, c := range f.calls {
		if strings.Contains(c, "cp -a /from/.") {
			copied = true
		}
		if strings.HasPrefix(c, "volume rm ") {
			removed = true
		}
	}
	if !copied {
		t.Fatal("nothing was copied:", f.calls)
	}
	if !removed {
		t.Fatal("the volume was not removed after the project let it go")
	}
}

// A copy that does not match leaves everything where it was. The volume is the
// only copy of a conversation at that moment, so this is the case that must not
// go wrong quietly.
func TestMigrationKeepsTheVolumeWhenTheCopyDiffers(t *testing.T) {
	m, p := migrating(t)
	before := p.Volume
	f := &migrationFake{from: "10 ./cxz.db\n20 ./sessions/x/events.jsonl\n", to: "10 ./cxz.db\n"}
	err := m.migrateProjectStateWith(context.Background(), p, f.run)
	if err == nil {
		t.Fatal("a copy that lost a file was accepted")
	}
	if !strings.Contains(err.Error(), "untouched") {
		t.Fatal("the error does not say the volume is untouched:", err)
	}
	if p.Volume != before {
		t.Fatal("the project stopped referring to a volume it still needs")
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "volume rm ") {
			t.Fatal("the volume was removed after a copy that did not match")
		}
	}
}

// A failure anywhere before the comparison is the same answer: nothing moved.
func TestMigrationLeavesTheVolumeWhenTheCopyFails(t *testing.T) {
	m, p := migrating(t)
	before := p.Volume
	f := &migrationFake{fail: "cp -a /from/."}
	if err := m.migrateProjectStateWith(context.Background(), p, f.run); err == nil {
		t.Fatal("a failed copy was accepted")
	}
	if p.Volume != before {
		t.Fatal("the project let go of its volume after a failed copy")
	}
}

// A project that already keeps its state in the manager's volume has nothing to
// move, and neither does an installation whose manager has no volume to share.
func TestMigrationIsNothingToDoTwice(t *testing.T) {
	m, p := migrating(t)
	p.Volume = ""
	f := &migrationFake{}
	if err := m.migrateProjectStateWith(context.Background(), p, f.run); err != nil || len(f.calls) != 0 {
		t.Fatal("a migrated project was migrated again", err, f.calls)
	}
	m2, p2 := migrating(t)
	m2.StateVolume = ""
	f = &migrationFake{}
	if err := m2.migrateProjectStateWith(context.Background(), p2, f.run); err != nil || len(f.calls) != 0 {
		t.Fatal("a manager with no volume of its own tried to move a project into it", err, f.calls)
	}
	if p2.Volume == "" {
		t.Fatal("the project lost its volume with nowhere to put the state")
	}
}

// What a new project gets depends on whether the manager has a volume to keep
// it in. There is one naming rule and it lives in one place.
func TestANewProjectHasNoVolumeOfItsOwn(t *testing.T) {
	m, _ := migrating(t)
	if v := m.newProjectVolume("abc"); v != "" {
		t.Fatal("a new project was given its own volume:", v)
	}
	m.StateVolume = ""
	if v := m.newProjectVolume("abc"); v == "" || !strings.Contains(v, "abc") {
		t.Fatal("a project on an older installation got no volume:", v)
	}
}

// The subpath a project container mounts and the directory the manager writes
// are the same place, said two ways.
func TestProjectStateIsOnePlace(t *testing.T) {
	m, _ := migrating(t)
	dir, subpath := m.projectState("abc")
	if !strings.HasSuffix(dir, "/projects/abc/state") {
		t.Fatal(dir)
	}
	if subpath != "projects/abc/state" {
		t.Fatal(subpath)
	}
	if !strings.HasPrefix(dir, m.Root) {
		t.Fatal("the directory is not under the manager's root:", dir)
	}
}
