package installer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webconfig"
)

func webFixture(t *testing.T) (transport.Installation, webconfig.Config, string) {
	t.Helper()
	root := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"test"}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := x509.MarshalECPrivateKey(key)
	c := webconfig.Config{Listen: "127.0.0.1:7350", Origin: "https://test:7350", Certificate: filepath.Join(root, "cert"), Key: filepath.Join(root, "key"), TokenFile: filepath.Join(root, "token")}
	os.WriteFile(c.Certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	os.WriteFile(c.Key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv}), 0600)
	os.WriteFile(c.TokenFile, []byte(strings.Repeat("a", 64)), 0600)
	v := transport.Installation{Owner: "fixture", Container: "manager", Image: "fixture:new", StateVolume: "fixture-state"}
	return v, c, root
}

func TestWebContainerIsolation(t *testing.T) {
	v, c, _ := webFixture(t)
	args, err := webArgs(v, c)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--restart unless-stopped", "--publish 127.0.0.1:7350:7350", "volume-subpath=run,readonly", "--read-only", "--cap-drop=ALL", "cxz.role=web", "--state /var/lib/cxz -x _web-serve"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
	for _, bad := range []string{"docker.sock", "target=/var/lib/cxz,", "aaaaaaaa"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unsafe argument %s", bad)
		}
	}
	c.Listen = "[::1]:7443"
	args, err = webArgs(v, c)
	if err != nil || !strings.Contains(strings.Join(args, " "), "[::1]:7443:7350") {
		t.Fatalf("%v %v", args, err)
	}
	for _, bad := range []string{":0", ":65536", "localhost:7350", "7350"} {
		c.Listen = bad
		if _, err = webArgs(v, c); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

// A plaintext gateway carries no certificate to mount or name, and is only safe
// because the port it is published on cannot be reached from a network. The
// publish address is where that has to be checked: inside its own namespace the
// gateway binds every interface.
func TestPlaintextWebPublishesOnlyOnLoopback(t *testing.T) {
	v, c, _ := webFixture(t)
	c.Certificate, c.Key = "", ""
	c.Origin = "http://127.0.0.1:7350"
	args, err := webArgs(v, c)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--publish 127.0.0.1:7350:7350", "target=/web/token,readonly", "--origin http://127.0.0.1:7350"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
	for _, bad := range []string{"--tls-cert", "--tls-key", "certificate.pem", "key.pem"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("plaintext gateway still carries %s: %s", bad, joined)
		}
	}
	for _, exposed := range []string{"0.0.0.0:7350", "192.0.2.10:7350", ":7350"} {
		c.Listen = exposed
		if _, err = webArgs(v, c); err == nil {
			t.Fatalf("published plaintext on %s", exposed)
		}
	}
}

func fakeWebDocker(t *testing.T, root string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(root, "calls")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("WEB_TEST_ROOT", root)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$WEB_TEST_ROOT/calls"
case "$1" in
 ps) if [ ! -e "$WEB_TEST_ROOT/renamed" ] && [ "$WEB_TEST_MISSING" != 1 ]; then echo old; fi;;
 inspect) printf '[{"Id":"old","Config":{"Image":"fixture:old","Labels":{"cxz.owner":"%s","cxz.role":"web"}},"State":{"Running":%s}}]' "${WEB_TEST_OWNER:-fixture}" "${WEB_TEST_RUNNING:-true}";;
 rename) if [ "$3" = manager-web ]; then rm -f "$WEB_TEST_ROOT/renamed"; else touch "$WEB_TEST_ROOT/renamed"; fi;;
 run) if [ "$WEB_TEST_UNSUPPORTED" = 1 ]; then exit 1; fi
      if [ "$2" = -d ] && [ "$WEB_TEST_FAIL" = 1 ]; then exit 1; fi;;
 *) ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return log
}

func TestWebRefreshLifecycle(t *testing.T) {
	for _, scenario := range []string{"uninstalled", "missing", "stopped", "foreign", "success", "unsupported", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			v, c, root := webFixture(t)
			log := fakeWebDocker(t, root)
			if err := core.WriteJSON(filepath.Join(root, "installation.json"), v); err != nil {
				t.Fatal(err)
			}
			if scenario != "uninstalled" {
				if err := core.WriteJSON(filepath.Join(root, "web-installation.json"), c); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "missing":
				t.Setenv("WEB_TEST_MISSING", "1")
			case "stopped":
				t.Setenv("WEB_TEST_RUNNING", "false")
			case "foreign":
				t.Setenv("WEB_TEST_OWNER", "other")
			case "unsupported":
				t.Setenv("WEB_TEST_UNSUPPORTED", "1")
			case "rollback":
				t.Setenv("WEB_TEST_FAIL", "1")
			}
			err := RefreshWebLocked(context.Background(), root, io.Discard)
			wantErr := scenario == "foreign" || scenario == "unsupported" || scenario == "rollback"
			if (err != nil) != wantErr {
				t.Fatalf("err=%v", err)
			}
			b, _ := os.ReadFile(log)
			calls := string(b)
			if scenario == "success" {
				if !strings.Contains(calls, "exec manager-web curl") || !strings.Contains(calls, "rm old") {
					t.Fatal(calls)
				}
			} else if scenario == "rollback" {
				if !strings.Contains(calls, "rename old manager-web\nstart old") {
					t.Fatal(calls)
				}
			} else if strings.Contains(calls, "rename ") || strings.Contains(calls, "stop ") || strings.Contains(calls, "run -d") {
				t.Fatal(calls)
			}
		})
	}
}
