package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/networkservices"
)

// Opt-in, disposable containers only. Set CXZ_TEST_WEB_DOCKER=1 to build a
// fixture, or supply CXZ_TEST_WEB_IMAGE with cxz, curl and /bin/true.
func TestWebDocker(t *testing.T) {
	image := os.Getenv("CXZ_TEST_WEB_IMAGE")
	if image == "" && os.Getenv("CXZ_TEST_WEB_DOCKER") == "1" {
		image = buildWebFixtureImage(t)
	}
	if image == "" {
		t.Skip("set CXZ_TEST_WEB_DOCKER=1 for container integration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 240*time.Second)
	defer cancel()
	v, c, root := webFixture(t)
	v.Owner = core.ID()
	v.Container = "cxz-web-test-" + v.Owner[:12]
	v.StateVolume = v.Container + "-state"
	v.Image = image
	shared, err := os.MkdirTemp(os.Getenv("CXZ_TEST_WEB_FILES_ROOT"), "cxz-web-files-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(shared)
	for _, p := range []*string{&c.Certificate, &c.Key, &c.TokenFile} {
		b, err := os.ReadFile(*p)
		if err != nil {
			t.Fatal(err)
		}
		*p = filepath.Join(shared, filepath.Base(*p))
		if err = os.WriteFile(*p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Allocate a host port for the fixture (the CLI requires a stable nonzero port).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.Listen = ln.Addr().String()
	ln.Close()
	c.Origin = "https://" + c.Listen
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cs, _ := dockerx.List(cleanup, "label=cxz.owner="+v.Owner)
		for _, container := range cs {
			dockerx.Run(cleanup, "rm", "-f", container.ID)
		}
		dockerx.Run(cleanup, "volume", "rm", v.StateVolume)
	}()

	// Stage only fixture files when the test runner and engine have separate filesystems.
	filesContainer := v.Container + "-files"
	if _, err = dockerx.Run(ctx, "run", "-d", "--name", filesContainer, "--label", "cxz.owner="+v.Owner, "-v", shared+":/fixture", "--entrypoint", "sh", image, "-c", "sleep 120"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		dockerx.Run(cleanup, "exec", filesContainer, "rm", "-f", "/fixture/cert", "/fixture/key", "/fixture/token")
		dockerx.Run(cleanup, "rm", "-f", filesContainer)
		dockerx.Run(cleanup, "run", "--rm", "--entrypoint", "rmdir", "-v", filepath.Dir(shared)+":/parent", image, "/parent/"+filepath.Base(shared))
	}()
	for _, p := range []string{c.Certificate, c.Key, c.TokenFile} {
		if _, err = dockerx.Run(ctx, "cp", p, filesContainer+":/fixture/"+filepath.Base(p)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = dockerx.Run(ctx, "volume", "create", v.StateVolume); err != nil {
		t.Fatal(err)
	}
	startManager := func(image string) {
		t.Helper()
		if _, err = dockerx.Run(ctx, "run", "-d", "--name", v.Container, "--label", "cxz.owner="+v.Owner, "--label", "cxz.role=daemon", "--mount", "type=volume,source="+v.StateVolume+",target=/var/lib/cxz", "-v", "/var/run/docker.sock:/var/run/docker.sock", "-e", "CXZ_AUTO_UPDATE=0", image, "--state", "/var/lib/cxz", "-x", "manager", "serve", "--agent", "/bin/true"); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err = dockerx.Run(ctx, "exec", v.Container, "cxz", "--state", "/var/lib/cxz", "_ready"); err == nil {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("Manager failed:", err)
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	startManager(image)
	if err = core.WriteJSON(filepath.Join(root, "installation.json"), v); err != nil {
		t.Fatal(err)
	}
	if err = InstallWeb(ctx, root, c, io.Discard); err != nil {
		t.Fatal(err)
	}
	first, _, err := findWeb(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	for _, mount := range first.Mounts {
		if mount.Destination == "/var/run/docker.sock" || mount.Destination == "/var/lib/cxz" {
			t.Fatalf("unsafe mount %+v", mount)
		}
	}
	request := func(path string, extra ...string) ([]byte, error) {
		args := []string{"exec", v.Container + "-web", "curl", "--silent", "--show-error", "--fail", "--insecure", "--header", "Host: " + strings.TrimPrefix(c.Origin, "https://"), "--header", "Origin: " + c.Origin, "https://127.0.0.1:7350" + path}
		return dockerx.Run(ctx, append(args, extra...)...)
	}
	b, err := request("/auth/login", "--include", "--header", "Content-Type: application/json", "--data", fmt.Sprintf(`{"token":%q}`, strings.Repeat("a", 64)))
	if err != nil {
		t.Fatal(err)
	}
	cookie := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.ToLower(line), "set-cookie:") {
			cookie = strings.Split(strings.TrimSpace(strings.SplitN(line, ":", 2)[1]), ";")[0]
		}
	}
	if cookie == "" {
		t.Fatal("login did not set cookie")
	}
	b, err = request("/cxz.SessionService/List", "--header", "Content-Type: application/json", "--header", "Connect-Protocol-Version: 1", "--cookie", cookie, "--data", "{}")
	if err != nil {
		t.Fatalf("Connect through Manager socket: %v %s", err, b)
	}
	// Exercise the automatic path with both TLS web and mTLS relay installed.
	ln, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relayListen := ln.Addr().String()
	ln.Close()
	if err = InstallRelay(ctx, root, RelayConfig{Listen: relayListen}, io.Discard); err != nil {
		t.Fatal(err)
	}
	relayBefore, _, err := findRelay(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	config := func(id string) []byte {
		t.Helper()
		b, err := dockerx.Run(ctx, "inspect", "--format", "{{json .HostConfig}}", id)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	webConfig, relayConfig := config(first.ID), config(relayBefore.ID)
	hostBefore, err := os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	nextImage := "cxz-web-test-next:" + v.Owner
	if _, err = dockerx.Run(ctx, "tag", v.Image, nextImage); err != nil {
		t.Fatal(err)
	}
	defer func() { dockerx.Run(context.Background(), "image", "rm", nextImage) }()
	// An automatic rollout changes the running image, leaving host metadata old.
	if _, err = dockerx.Run(ctx, "stop", "--time", "1", v.Container); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "rm", v.Container); err != nil {
		t.Fatal(err)
	}
	startManager(nextImage)
	webState, err := WebStatus(ctx, root, c, "")
	if err != nil || !webState.NeedsRefresh || webState.ManagerImage != nextImage {
		t.Fatal("stale web status", webState, err)
	}
	relayState, err := RelayStatus(ctx, root)
	if err != nil || !relayState.NeedsRefresh {
		t.Fatal("stale relay status", relayState, err)
	}
	refresh := func() ([]byte, error) {
		return dockerx.Run(ctx, "exec", v.Container, "cxz", "_refresh-network-services", "/var/lib/cxz", v.Container, v.Owner)
	}
	_, release, err := networkservices.LockHost(ctx, v.Container, v.Owner)
	if err != nil {
		t.Fatal(err)
	}
	out, blocked := refresh()
	release()
	if blocked == nil || !strings.Contains(blocked.Error(), "busy") {
		t.Fatalf("host lock bypassed: %s %v", out, blocked)
	}
	out, err = refresh()
	if err != nil {
		t.Fatalf("automatic refresh: %v %s", err, out)
	}
	var report networkservices.Report
	if err = json.Unmarshal(out, &report); err != nil || report.Failure() != nil || report.Web.NeedsRefresh || report.Relay.NeedsRefresh {
		t.Fatalf("refresh report: %s %v", out, err)
	}
	relayAfter, _, err := findRelay(ctx, v)
	if err != nil || relayAfter.ID == relayBefore.ID || relayAfter.Config.Image != nextImage {
		t.Fatal("relay not refreshed", err)
	}
	webAfter, _, err := findWeb(ctx, v)
	if err != nil || webAfter.ID == first.ID || webAfter.Config.Image != nextImage {
		t.Fatal("web not refreshed", err)
	}
	if !bytes.Equal(webConfig, config(webAfter.ID)) || !bytes.Equal(relayConfig, config(relayAfter.ID)) {
		t.Fatal("automatic refresh changed HostConfig")
	}
	hostAfter, err := os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil || !bytes.Equal(hostBefore, hostAfter) {
		t.Fatal("automatic refresh touched host metadata", err)
	}
	if _, err = refresh(); err != nil {
		t.Fatal("repeated automatic refresh", err)
	}
	if err = UninstallRelay(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err = RefreshWebLocked(ctx, root, io.Discard); err != nil {
		t.Fatal(err)
	}
	second, _, err := findWeb(ctx, v)
	if err != nil || first.ID == second.ID {
		t.Fatalf("gateway not replaced: %v", err)
	}
	if err = RefreshWebLocked(ctx, root, io.Discard); err != nil {
		t.Fatal(err)
	}
	unchanged, _, err := findWeb(ctx, v)
	if err != nil || unchanged.ID != second.ID {
		t.Fatalf("refresh retry restarted current web: %v", err)
	}
	if _, err = request("/auth/status", "--cookie", cookie); err == nil {
		t.Fatal("old browser session survived gateway replacement")
	}
	if _, err = dockerx.Run(ctx, "stop", v.Container+"-web"); err != nil {
		t.Fatal(err)
	}
	if err = RefreshWebLocked(ctx, root, io.Discard); err != nil {
		t.Fatal(err)
	}
	stopped, _, err := findWeb(ctx, v)
	if err != nil || stopped.State.Running {
		t.Fatalf("stopped gateway restarted: %v", err)
	}
	if err = UninstallWeb(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, found, err := findWeb(ctx, v); err != nil || found {
		t.Fatalf("web still installed: %v", err)
	}

	// The same gateway with no certificate: the desktop default. Nothing is
	// mounted for TLS, the readiness probe follows the origin's scheme, and a
	// browser signs in over the loopback port exactly as it does over HTTPS.
	plain := c
	plain.Certificate, plain.Key = "", ""
	plain.Origin = "http://" + plain.Listen
	if err = InstallWeb(ctx, root, plain, io.Discard); err != nil {
		t.Fatal(err)
	}
	gateway, _, err := findWeb(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	for _, mount := range gateway.Mounts {
		if strings.Contains(mount.Destination, "certificate.pem") || strings.Contains(mount.Destination, "key.pem") {
			t.Fatalf("plaintext gateway mounted TLS material: %+v", mount)
		}
	}
	plainRequest := func(path string, extra ...string) ([]byte, error) {
		args := []string{"exec", v.Container + "-web", "curl", "--silent", "--show-error", "--fail", "--header", "Host: " + plain.Listen, "--header", "Origin: " + plain.Origin, "http://127.0.0.1:7350" + path}
		return dockerx.Run(ctx, append(args, extra...)...)
	}
	if b, err = plainRequest("/auth/login", "--include", "--header", "Content-Type: application/json", "--data", fmt.Sprintf(`{"token":%q}`, strings.Repeat("a", 64))); err != nil {
		t.Fatalf("plaintext login: %v %s", err, b)
	}
	if !strings.Contains(strings.ToLower(string(b)), "set-cookie:") {
		t.Fatalf("plaintext login set no cookie: %s", b)
	}
	if strings.Contains(strings.ToLower(string(b)), "strict-transport-security") {
		t.Fatalf("plaintext gateway promised HSTS: %s", b)
	}
	if err = UninstallWeb(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err = dockerx.Run(ctx, "exec", v.Container, "cxz", "--state", "/var/lib/cxz", "_ready"); err != nil {
		t.Fatal("uninstall interrupted Manager", err)
	}
}

func buildWebFixtureImage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, "cxz"), "./cmd/cxz")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build web fixture: %v %s", err, b)
	}
	// A minimal filesystem UI fixture is sufficient for gateway lifecycle tests.
	if err := os.Mkdir(filepath.Join(dir, "webui"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "webui", "index.html"), []byte("<!doctype html><title>Fixture</title>"), 0644); err != nil {
		t.Fatal(err)
	}
	dockerfile := "FROM docker:28-cli AS dockercli\nFROM alpine:3.22\nCOPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker\nRUN apk add --no-cache curl ca-certificates util-linux\nCOPY cxz /usr/local/bin/cxz\nCOPY webui /usr/local/share/cxz/webui\nENTRYPOINT [\"/usr/local/bin/cxz\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0600); err != nil {
		t.Fatal(err)
	}
	image := "cxz-web-test:" + core.ID()
	if _, err := dockerx.Run(t.Context(), "build", "-t", image, dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dockerx.Run(context.Background(), "image", "rm", image) })
	return image
}
