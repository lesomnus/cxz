package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/pki"
	"github.com/lesomnus/cxz/internal/transport"
)

// The relay is the one piece whose parts only meet in a container: the root is
// created by the manager, the leaf arrives through a volume subpath, the port is
// published by Docker, and the certificate that opens it is signed through a
// docker exec. This runs all of it and then makes a real mutual-TLS request.
//
// The request is made from inside a container, not from the test process. A
// published port belongs to the Docker engine's host, which is not necessarily
// the machine running the test -- that is exactly the arrangement a remote
// engine creates, and getting it wrong here is how the readiness probe was
// wrong before.
//
// Opt-in, disposable containers only, as TestWebDocker is.
func TestRelayDocker(t *testing.T) {
	image := os.Getenv("CXZ_TEST_WEB_IMAGE")
	if image == "" && os.Getenv("CXZ_TEST_WEB_DOCKER") == "1" {
		image = buildWebFixtureImage(t)
	}
	if image == "" {
		t.Skip("set CXZ_TEST_WEB_DOCKER=1 for container integration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	root := t.TempDir()
	v := transport.Installation{Owner: core.ID(), Image: image}
	v.Container = "cxz-relay-test-" + v.Owner[:12]
	v.StateVolume = v.Container + "-state"
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		cs, _ := dockerx.List(cleanup, "label=cxz.owner="+v.Owner)
		for _, container := range cs {
			dockerx.Run(cleanup, "rm", "-f", container.ID)
		}
		dockerx.Run(cleanup, "volume", "rm", v.StateVolume)
	}()
	if _, err := dockerx.Run(ctx, "volume", "create", v.StateVolume); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "run", "-d", "--name", v.Container, "--label", "cxz.owner="+v.Owner, "--mount", "type=volume,source="+v.StateVolume+",target=/var/lib/cxz", "-e", "CXZ_AUTO_UPDATE=0", image, "--state", "/var/lib/cxz", "-x", "manager", "serve", "--agent", "/bin/true"); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := dockerx.Run(ctx, "exec", v.Container, "cxz", "--state", "/var/lib/cxz", "_ready"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			logs, _ := dockerx.Run(context.Background(), "logs", v.Container)
			t.Fatalf("Manager failed: %s", logs)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err := core.WriteJSON(filepath.Join(root, "installation.json"), v); err != nil {
		t.Fatal(err)
	}
	cxz := func(args ...string) ([]byte, error) {
		return dockerx.Run(ctx, append([]string{"exec", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz"}, args...)...)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listen := ln.Addr().String()
	ln.Close()
	if err = InstallRelay(ctx, root, RelayConfig{Listen: listen}, io.Discard); err != nil {
		t.Fatal(err)
	}
	state, err := RelayStatus(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "running" || state.Listen != listen || !strings.HasPrefix(state.Root, "SHA256:") {
		t.Fatalf("%+v", state)
	}
	if state.NeedsRefresh {
		t.Fatal("a freshly installed relay reports a version skew")
	}
	relay, exists, err := findRelay(ctx, v)
	if err != nil || !exists {
		t.Fatal(err)
	}
	// The process on the network must not be able to issue certificates, read
	// the manager's database, or write anything at all.
	for _, mount := range relay.Mounts {
		if mount.Destination == "/var/lib/cxz" || mount.Destination == "/var/run/docker.sock" || mount.RW {
			t.Fatalf("relay mount is too broad: %+v", mount)
		}
	}
	if out, err := dockerx.Run(ctx, "exec", relay.ID, "sh", "-c", "cat /var/lib/cxz/pki/ca.key 2>&1 || true"); err == nil && bytes.Contains(out, []byte("PRIVATE KEY")) {
		t.Fatal("the relay container can read the installation root's private key")
	}
	address := ""
	for _, network := range relay.NetworkSettings.Networks {
		if network.IPAddress != "" {
			address = net.JoinHostPort(network.IPAddress, "7349")
		}
	}
	if address == "" {
		t.Fatal("the relay has no address another container can reach")
	}

	// Enrolling, exactly as `cxz expose ca` and `cxz expose sign` do it.
	caPEM, err := cxz("_pki", "ca")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, csrPEM, err := pki.NewRequest()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := dockerx.Output(ctx, bytes.NewReader(csrPEM), "exec", "-i", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_pki", "sign", "--label", "integration")
	if err != nil {
		t.Fatal(err)
	}
	client := stage(t, ctx, v.Container, "/client", map[string][]byte{"client.crt": certPEM, "client.key": keyPEM, "ca.crt": caPEM})
	out, err := cxz("_expose-check", "--address", address, "--identity", client)
	if err != nil {
		t.Fatal("mutual TLS did not reach the Manager:", err)
	}
	if !strings.Contains(string(out), "answered") {
		t.Fatal("the check did not report an answer:", string(out))
	}

	// A certificate from another installation is refused by the relay, so the
	// pin is doing something rather than merely being configured.
	elsewhere, err := pki.EnsureCA(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, foreignCSR, err := pki.NewRequest()
	if err != nil {
		t.Fatal(err)
	}
	foreignCert, _, err := elsewhere.SignClient(foreignCSR, "intruder", 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign := stage(t, ctx, v.Container, "/foreign", map[string][]byte{"client.crt": foreignCert, "client.key": foreignKey, "ca.crt": caPEM})
	if _, err = cxz("_expose-check", "--address", address, "--identity", foreign); err == nil {
		t.Fatal("a certificate from another installation reached the Manager")
	}

	// Revoking applies to the relay that is already running.
	var report struct {
		Issued []struct {
			Serial string `json:"serial"`
			Label  string `json:"label"`
		} `json:"issued"`
	}
	issued, err := cxz("_pki", "issued")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(issued, &report); err != nil {
		t.Fatal(err, string(issued))
	}
	serial := ""
	for _, r := range report.Issued {
		if r.Label == "integration" {
			serial = r.Serial
		}
	}
	if serial == "" {
		t.Fatal("the issued certificate was not recorded:", string(issued))
	}
	if _, err = cxz("_pki", "revoke", serial); err != nil {
		t.Fatal(err)
	}
	if _, err = cxz("_expose-check", "--address", address, "--identity", client); err == nil {
		t.Fatal("a revoked certificate still reached the Manager")
	}

	if err = UninstallRelay(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, found, err := findRelay(ctx, v); err != nil || found {
		t.Fatalf("relay still installed: %v", err)
	}
	if _, err = dockerx.Run(ctx, "exec", v.Container, "cxz", "--state", "/var/lib/cxz", "_ready"); err != nil {
		t.Fatal("removing the relay interrupted the Manager:", err)
	}
	// The root outlives the relay, so a client that enrolled stays enrolled.
	again, err := cxz("_pki", "ca")
	if err != nil || string(again) != string(caPEM) {
		t.Fatal("the installation root changed:", err)
	}
}

// stage writes files into a running container, which is how a test puts a
// client identity where a container can read it.
func stage(t *testing.T, ctx context.Context, container, dir string, files map[string][]byte) string {
	t.Helper()
	local := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(local, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dockerx.Run(ctx, "exec", container, "mkdir", "-p", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := dockerx.Run(ctx, "cp", local+"/.", fmt.Sprintf("%s:%s", container, dir)); err != nil {
		t.Fatal(err)
	}
	return dir
}
