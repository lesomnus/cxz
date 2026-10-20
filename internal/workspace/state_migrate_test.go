package workspace

import (
	"context"
	"os"
	"path/filepath"
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

// Everything that used to mount a project's volume has to ask one place where
// the state is, because for a migrated project there is no volume to mount. The
// first version of this left seven callers asking for `p.Volume` and a
// provision failed with "invalid volume name or ID: value is empty" -- after
// the state had already moved, which left the project without a container.
func TestNothingMountsAnEmptyVolume(t *testing.T) {
	m, p := migrating(t)

	// Still on its own volume: unchanged.
	mount := m.stateMount(p, "/cxz/state", false)
	if mount.volume != p.Volume || mount.subpath != "" || mount.project != p.ID {
		t.Fatal("a project with its own volume was mounted as something else", mount)
	}
	if loc := m.stateLocation(p); loc != "volume:"+p.Volume {
		t.Fatal(loc)
	}

	// Migrated: the manager's volume at this project's subpath, and never an
	// empty source.
	p.Volume = ""
	mount = m.stateMount(p, "/cxz/state", true)
	if mount.volume != m.StateVolume {
		t.Fatal("the mount source is", mount.volume)
	}
	if mount.subpath != "projects/"+p.ID+"/state" {
		t.Fatal("the subpath is", mount.subpath)
	}
	if !mount.write {
		t.Fatal("write was dropped")
	}
	// The manager's volume carries no project label, and the ownership check
	// compares against this field.
	if mount.project != "" {
		t.Fatal("the manager's volume was checked as a project's:", mount.project)
	}
	if loc := m.stateLocation(p); !strings.Contains(loc, m.StateVolume) || strings.HasSuffix(loc, ":") {
		t.Fatal("the location says nothing useful:", loc)
	}
}

// A migrated project's runtime.json is written and read by the manager itself,
// because it owns that filesystem. The rename is still what makes it atomic.
func TestRuntimeJSONRoundTripsWithoutAContainer(t *testing.T) {
	ctx := context.Background()
	m, p := migrating(t)
	p.Volume = ""
	dir, _ := m.projectState(p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	// Nothing written yet is not an error, it is nothing.
	b, err := m.readRuntime(ctx, p)
	if err != nil || len(b) != 0 {
		t.Fatal("an absent runtime.json was not empty", err, string(b))
	}
	if err = m.writeRuntime(ctx, p, Runtime{Token: "t0ken"}); err != nil {
		t.Fatal(err)
	}
	b, err = m.readRuntime(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "t0ken") {
		t.Fatal("read back", string(b))
	}
	if _, err = os.Stat(filepath.Join(dir, "runtime.next")); !os.IsNotExist(err) {
		t.Fatal("the temporary file was left behind")
	}
}
