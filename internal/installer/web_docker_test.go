package installer

import (
	"context"
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
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
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
	if _, err = dockerx.Run(ctx, "run", "-d", "--name", v.Container, "--label", "cxz.owner="+v.Owner, "--mount", "type=volume,source="+v.StateVolume+",target=/var/lib/cxz", "-e", "CXZ_AUTO_UPDATE=0", image, "--state", "/var/lib/cxz", "-x", "manager", "serve", "--agent", "/bin/true"); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = dockerx.Run(ctx, "exec", v.Container, "cxz", "--state", "/var/lib/cxz", "_ready"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			logs, _ := dockerx.Run(context.Background(), "logs", v.Container)
			t.Fatalf("Manager failed: %s %v", logs, err)
		case <-time.After(100 * time.Millisecond):
		}
	}
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
	// A Manager image change refreshes the gateway using saved configuration.
	nextImage := "cxz-web-test-next:" + v.Owner
	if _, err = dockerx.Run(ctx, "tag", v.Image, nextImage); err != nil {
		t.Fatal(err)
	}
	defer func() { dockerx.Run(context.Background(), "image", "rm", nextImage) }()
	v.Image = nextImage
	if err = core.WriteJSON(filepath.Join(root, "installation.json"), v); err != nil {
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
	dockerfile := "FROM alpine:3.22\nRUN apk add --no-cache curl ca-certificates\nCOPY cxz /usr/local/bin/cxz\nENTRYPOINT [\"/usr/local/bin/cxz\"]\n"
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
