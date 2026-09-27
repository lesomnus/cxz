package cxzupdate

import (
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCloneManagerDocker(t *testing.T) {
	if os.Getenv("CXZ_TEST_AUTO_UPDATE_DOCKER") != "1" {
		t.Skip("set CXZ_TEST_AUTO_UPDATE_DOCKER=1 for isolated Docker replacement verification")
	}
	// The development workspace reaches Docker over TCP. Present a temporary
	// local socket to exercise the production helper's Unix API transport.
	if host := os.Getenv("DOCKER_HOST"); strings.HasPrefix(host, "tcp://") && os.Getenv("DOCKER_TLS_VERIFY") == "" {
		socket := filepath.Join(t.TempDir(), "docker.sock")
		ln, e := net.Listen("unix", socket)
		if e != nil {
			t.Fatal(e)
		}
		defer ln.Close()
		go func() {
			for {
				c, e := ln.Accept()
				if e != nil {
					return
				}
				go func() {
					defer c.Close()
					d, e := net.Dial("tcp", strings.TrimPrefix(host, "tcp://"))
					if e != nil {
						return
					}
					defer d.Close()
					go io.Copy(d, c)
					_, _ = io.Copy(c, d)
				}()
			}
		}()
		t.Setenv("DOCKER_HOST", "unix://"+socket)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	owner := core.ID()
	name := "cxz-update-test-" + owner
	project := name + "-project"
	volume := name + "-state"
	network := name + "-network"
	run := func(args ...string) string {
		t.Helper()
		b, e := dockerx.Run(ctx, args...)
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(string(b))
	}
	run("volume", "create", volume)
	defer dockerx.Run(context.Background(), "volume", "rm", volume)
	run("network", "create", network)
	defer dockerx.Run(context.Background(), "network", "rm", network)
	projectID := run("run", "-d", "--name", project, "--network", network, "alpine:3.22", "sleep", "120")
	defer dockerx.Run(context.Background(), "rm", "-f", project)
	oldID := run("run", "-d", "--name", name, "--network", network, "--network-alias", "manager", "--label", "cxz.role=daemon", "--label", "cxz.owner="+owner, "--mount", "type=volume,source="+volume+",target=/state", "-e", "CXZ_MANAGER_IMAGE=old", "-e", "PRESERVE_ME=yes", "alpine:3.22", "sleep", "120")
	defer dockerx.Run(context.Background(), "rm", "-f", oldID)
	run("exec", oldID, "sh", "-c", "echo preserved > /state/identity")
	run("stop", oldID)
	run("rename", oldID, name+"-previous")
	newID, e := CloneManager(ctx, oldID, name, owner, "alpine:3.22")
	if e != nil {
		t.Fatal(e)
	}
	defer dockerx.Run(context.Background(), "rm", "-f", newID)
	run("start", newID)
	if got := run("exec", newID, "sh", "-c", "cat /state/identity; echo $PRESERVE_ME:$CXZ_MANAGER_IMAGE"); got != "preserved\nyes:alpine:3.22" {
		t.Fatal(got)
	}
	p, e := dockerx.Inspect(ctx, project)
	if e != nil || p.ID != projectID || !p.State.Running {
		t.Fatal("project replaced or stopped")
	}
	if _, e = CloneManager(ctx, oldID, name+"-foreign", "wrong-owner", "alpine:3.22"); e == nil {
		t.Fatal("foreign manager cloned")
	}
	// Manual installation and automatic replacement reserve the same Docker name.
	release, e := ReserveInstall(ctx, name, owner, "alpine:3.22")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if _, e = ReserveInstall(ctx, name, owner, "alpine:3.22"); e == nil {
		t.Fatal("duplicate installer admitted")
	}
}
