package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/memoryview"
)

func TestLiveRetainedMemoryWithoutProjectContainer(t *testing.T) {
	if os.Getenv("CXZ_TEST_MEMORY") != "1" {
		t.Skip("set CXZ_TEST_MEMORY=1 and CXZ_TEST_BINARY to test disposable retained volumes")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	binary, err := filepath.Abs(os.Getenv("CXZ_TEST_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
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
	m.Owner = core.ID()
	m.Image = "docker:cli"
	m.ToolsVolume = "cxz-memory-test-" + m.Owner + "-tools"
	projects := []*Project{{ID: core.ID()}, {ID: core.ID()}}
	helper := "cxz-memory-seed-" + m.Owner
	var volumes []string
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		dockerx.Run(cleanup, "rm", "-f", helper)
		for _, v := range volumes {
			if _, err := dockerx.Run(cleanup, "volume", "rm", v); err != nil {
				t.Error(err)
			}
		}
	})
	for i, p := range projects {
		p.Volume = "cxz-memory-test-" + m.Owner + "-" + p.ID
		account := "default"
		if i == 1 {
			account = "other"
		}
		p.Sessions = []*api.Session{{Id: core.ID(), CreateId: core.ID(), Agent: "claude", Account: account, State: "stopped", ProjectId: p.ID}}
		if err = m.save(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err = dockerx.EnsureResource(ctx, "volume", p.Volume, m.Owner, p.ID); err != nil {
			t.Fatal(err)
		}
		volumes = append(volumes, p.Volume)
	}
	if err = dockerx.EnsureResource(ctx, "volume", m.ToolsVolume, m.Owner, ""); err != nil {
		t.Fatal(err)
	}
	volumes = append(volumes, m.ToolsVolume)
	if _, err = dockerx.Run(ctx, "run", "-d", "--name", helper, "--network", "none", "--mount", "type=volume,source="+projects[0].Volume+",target=/source", "--mount", "type=volume,source="+projects[1].Volume+",target=/target", "--mount", "type=volume,source="+m.ToolsVolume+",target=/tools", "--entrypoint", "sleep", m.Image, "120"); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "cp", binary, helper+":/tools/cxz"); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for i, p := range projects {
		mount := "source"
		if i == 1 {
			mount = "target"
		}
		s := p.Sessions[0]
		profile := accounts.Config(accounts.SessionRoot("data", s.CreateId), s.Account)
		files := map[string]string{
			filepath.Join(profile, ".credentials.json"):                "keep-login",
			filepath.Join(filepath.Dir(profile), "login.lock"):         "",
			filepath.Join("data", "sessions", s.Id, "supervisor.lock"): "",
		}
		if i == 0 {
			files[filepath.Join(profile, "projects/old/memory/MEMORY.md")] = "saved across projects and accounts"
		}
		for name, content := range files {
			if err = tw.WriteHeader(&tar.Header{Name: mount + "/" + name, Mode: 0600, Size: int64(len(content)), Uid: 1000 + i, Gid: 1000 + i}); err != nil {
				t.Fatal(err)
			}
			if _, err = tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err = dockerx.Input(ctx, &archive, "exec", "-i", helper, "tar", "xf", "-", "-C", "/"); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", helper, "sh", "-c", "chown -R 1000:1000 /source/data && chown -R 1001:1001 /target/data && chmod -R go-rwx /source/data /target/data"); err != nil {
		t.Fatal(err)
	}
	// Remove the only seed process: browsing/copying needs no project runtime.
	if _, err = dockerx.Run(ctx, "rm", "-f", helper); err != nil {
		t.Fatal(err)
	}
	source, target := projects[0].Sessions[0], projects[1].Sessions[0]
	req := &api.MemoryRequest{SessionId: source.Id, Path: "projects/old/memory/MEMORY.md"}
	out, err := m.Memory(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var page memoryview.Page
	if err = json.Unmarshal(out.Data, &page); err != nil || page.Content != "saved across projects and accounts" || !strings.HasPrefix(page.Location, "volume:") {
		t.Fatal(page, err)
	}
	copyReq := &api.CopyMemoryRequest{SessionId: source.Id, Path: "projects/old/memory", TargetId: target.Id, TargetPath: "projects/new/memory"}
	if _, err = m.CopyMemory(ctx, copyReq); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CopyMemory(ctx, copyReq); err == nil {
		t.Fatal("overwrote existing destination")
	}
	out, err = m.Memory(ctx, &api.MemoryRequest{SessionId: target.Id, Path: "projects/new/memory/MEMORY.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(out.Data, &page); err != nil || page.Content != "saved across projects and accounts" {
		t.Fatal(page, err)
	}
	targetConfig := accounts.Config(accounts.SessionRoot("/state/data", target.CreateId), target.Account)
	// The remote user's UID can read copied data, including new parent directories.
	if _, err = dockerx.Run(ctx, "run", "--rm", "--user", "1001:1001", "--network", "none", "--mount", "type=volume,source="+projects[1].Volume+",target=/state,readonly", "--entrypoint", "cat", m.Image, filepath.Join(targetConfig, "projects/new/memory/MEMORY.md"), filepath.Join(targetConfig, ".credentials.json")); err != nil {
		t.Fatal("target ownership or login damaged", err)
	}
	m.Owner = "foreign"
	if _, err = m.Memory(ctx, req); err == nil {
		t.Fatal("read unowned volume")
	}
}
