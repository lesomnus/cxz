package versionuse

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostSwitchDocker(t *testing.T) {
	image := os.Getenv("CXZ_TEST_USE_IMAGE")
	if image == "" {
		t.Skip("set CXZ_TEST_USE_IMAGE to an isolated fixture image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	workspace := "/workspaces/cxz-use-test-" + core.ID()
	if e := os.MkdirAll(workspace, 0700); e != nil {
		t.Fatal(e)
	}
	defer os.Remove(workspace)
	if _, e := dockerx.Run(ctx, "run", "--rm", "-v", workspace+":/work", "alpine:3.22", "true"); e != nil {
		t.Fatal(e)
	}
	defer dockerx.Run(context.Background(), "run", "--rm", "-v", "/workspaces:/workspaces", "alpine:3.22", "rmdir", workspace)
	b, e := dockerx.Run(ctx, "run", "--rm", image, "--format", "json", "version")
	if e != nil {
		t.Fatal(e)
	}
	var v struct{ Version, Revision string }
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	p := versionpin.Pin{Channel: os.Getenv("CXZ_TEST_USE_CHANNEL"), Version: v.Version, Revision: v.Revision, Image: image, Generation: core.ID()}
	// A pending local pin avoids copying any real host GitHub credentials.
	if e = versionpin.Save(root, p); e != nil {
		t.Fatal(e)
	}
	defer func() {
		install, e := transport.Load(root)
		if e != nil {
			return
		}
		containers, _ := dockerx.List(context.Background(), "label=cxz.owner="+install.Owner)
		for _, c := range containers {
			dockerx.Run(context.Background(), "rm", "-f", c.ID)
		}
		for _, vol := range []string{install.StateVolume, install.ToolsVolume} {
			dockerx.Run(context.Background(), "volume", "rm", vol)
		}
	}()
	if e = installer.InstallLocked(ctx, root, workspace, image, true, io.Discard); e != nil {
		t.Fatal(e)
	}
	before, e := transport.Load(root)
	if e != nil {
		t.Fatal(e)
	}
	original, e := dockerx.Inspect(ctx, before.Container)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := Prepare(ctx, root, p, io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	if e = Apply(ctx, root, tx, io.Discard); e != nil {
		t.Fatal(e)
	}
	if e = Apply(ctx, root, tx, io.Discard); e != nil {
		t.Fatal(e)
	}
	if e = Finish(ctx, root, tx); e != nil {
		t.Fatal(e)
	}
	after, e := dockerx.Inspect(ctx, before.Container)
	if e != nil || after.ID == original.ID || !after.State.Running {
		t.Fatal("manager not replaced", e)
	}
	backup, e := dockerx.Inspect(ctx, original.ID)
	if e != nil || backup.State.Running {
		t.Fatal("old manager not retained", e)
	}
	b, e = dockerx.Run(ctx, "exec", after.ID, "cat", "/var/lib/cxz/version-pin.json")
	if e != nil {
		t.Fatal(e)
	}
	var pinned versionpin.Pin
	if e = json.Unmarshal(b, &pinned); e != nil || !pinned.Ready || pinned.Version != p.Version || pinned.Channel != p.Channel {
		t.Fatal("server pin", string(b), e)
	}
	p.Ready = true
	if e = versionpin.Save(root, p); e != nil {
		t.Fatal(e)
	}
	if e = Clear(ctx, root); e != nil {
		t.Fatal(e)
	}
	if e = versionpin.Clear(root); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(root, "version-pin.json")); p.Channel == "" && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if p.Channel != "" {
		if c, e := versionpin.Channel(root); e != nil || c != p.Channel {
			t.Fatal("channel lost", c, e)
		}
	}
	t.Log("manager replaced; old container retained; server/client pin set and removed; retry idempotent")
}

func TestDatabasePreflightDoesNotDowngrade(t *testing.T) {
	root := t.TempDir()
	db, e := sql.Open("sqlite3", filepath.Join(root, "cxz.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec("PRAGMA user_version=2"); e != nil {
		t.Fatal(e)
	}
	if e = validateDatabase(root); e == nil {
		t.Fatal("newer schema accepted")
	}
	var v int
	if e = db.QueryRow("PRAGMA user_version").Scan(&v); e != nil || v != 2 {
		t.Fatal("schema modified", v, e)
	}
}
