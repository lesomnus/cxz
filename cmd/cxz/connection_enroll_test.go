package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/enroll"
	"github.com/lesomnus/cxz/internal/pki"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli/xlitest"
)

func enrollTestState(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := `{"connections":{"default":"work","work":{"target":"ssh://alice@work:2222"},"lab":{"target":"mtls://10.0.0.5:7349"},"tcp":{"target":"tcp://127.0.0.1:7349","token_file":"missing-secret-file"}}}`
	if err := os.WriteFile(settings.Path(root), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

// The transport column reports what the client will use, which is not what the
// target says: an enrolled ssh connection carries nothing but enrollments.
func TestConnectionListReportsTheTransport(t *testing.T) {
	root := enrollTestState(t)
	transportOf := func() map[string]string {
		got := xlitest.Run(t, newRoot(root), "connection", "ls", "--format", "json")
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		var rows []struct {
			Name      string `json:"name"`
			Transport string `json:"transport"`
			Detail    string `json:"transport_detail"`
		}
		if err := json.Unmarshal([]byte(got.Stdout), &rows); err != nil {
			t.Fatal(err, got.Stdout)
		}
		out := map[string]string{}
		for _, r := range rows {
			out[r.Name] = r.Transport + "|" + r.Detail
		}
		return out
	}
	before := transportOf()
	if before["work"] != "ssh|not enrolled" {
		t.Fatalf("%+v", before)
	}
	if !strings.HasPrefix(before["lab"], "mtls|not enrolled") {
		t.Fatalf("%+v", before)
	}
	if before["tcp"] != "tcp|plaintext; token" {
		t.Fatalf("%+v", before)
	}

	authority := signer(t)
	enrollByHand(t, root, "work", authority, "10.0.0.9:7349")
	after := transportOf()
	if !strings.HasPrefix(after["work"], "mtls|expires ") {
		t.Fatalf("an enrolled ssh connection is still reported as ssh: %+v", after)
	}
	table := xlitest.Run(t, newRoot(root), "connection", "ls")
	if table.Err != nil || !strings.Contains(table.Stdout, "TRANSPORT") || !strings.Contains(table.Stdout, "mtls (expires") {
		t.Fatalf("%q %v", table.Stdout, table.Err)
	}
	// The target is still what was configured: enrolling does not rewrite it.
	if !strings.Contains(table.Stdout, "ssh://alice@work:2222") {
		t.Fatal("the configured target disappeared:", table.Stdout)
	}
}

// The manual path has to work without ssh, and it must never ask for the key:
// a request goes out, a certificate comes back, and the key stays here.
func TestManualEnrollmentThroughTheCLI(t *testing.T) {
	root := enrollTestState(t)
	authority := signer(t)
	request := xlitest.Run(t, newRoot(root), "connection", "enroll", "--csr", "lab")
	if request.Err != nil {
		t.Fatal(request.Err)
	}
	if !strings.Contains(request.Stdout, "CERTIFICATE REQUEST") {
		t.Fatal("no request was printed:", request.Stdout)
	}
	if strings.Contains(request.Stdout, "PRIVATE KEY") {
		t.Fatal("the private key was printed")
	}
	cert, _, err := authority.SignClient([]byte(request.Stdout), "desk", 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, caPath := filepath.Join(dir, "client.crt"), filepath.Join(dir, "ca.crt")
	os.WriteFile(certPath, cert, 0600)
	os.WriteFile(caPath, authority.CertificatePEM(), 0600)

	// A certificate with no root to pin is half an identity, and so is a root
	// with no certificate.
	if got := xlitest.Run(t, newRoot(root), "connection", "enroll", "--certificate", certPath, "lab"); got.Err == nil {
		t.Fatal("accepted a certificate without a root")
	}
	if got := xlitest.Run(t, newRoot(root), "connection", "enroll", "--ca", caPath, "lab"); got.Err == nil {
		t.Fatal("accepted a root without a certificate")
	}
	done := xlitest.Run(t, newRoot(root), "connection", "enroll", "--certificate", certPath, "--ca", caPath, "lab")
	if done.Err != nil {
		t.Fatal(done.Err)
	}
	if !strings.Contains(done.Stderr, "mtls://10.0.0.5:7349") {
		t.Fatal("the address the connection will dial was not reported:", done.Stderr)
	}
	stored, ok, err := enroll.Load(root, "lab")
	if err != nil || !ok || !stored.Usable() {
		t.Fatal(stored, ok, err)
	}
	if stored.Address != "10.0.0.5:7349" {
		t.Fatal("the mtls target was not used as the address:", stored.Address)
	}
	// An ssh connection has no address in its target, so the manual path needs
	// to be told one rather than guessing a port.
	if got := xlitest.Run(t, newRoot(root), "connection", "enroll", "--certificate", certPath, "--ca", caPath, "work"); got.Err == nil {
		t.Fatal("completed a manual enrollment with nowhere to dial")
	}
	forget := xlitest.Run(t, newRoot(root), "connection", "forget", "lab")
	if forget.Err != nil {
		t.Fatal(forget.Err)
	}
	if _, ok, _ = enroll.Load(root, "lab"); ok {
		t.Fatal("forgetting left the identity behind")
	}
}

func TestEnrollAndCheckRefusals(t *testing.T) {
	root := enrollTestState(t)
	for _, args := range [][]string{
		{"connection", "enroll", "missing"},
		{"connection", "enroll", "tcp"},
		{"connection", "enroll", "lab"},
		{"connection", "check", "work"},
		{"connection", "check", "missing"},
		{"connection", "forget", "work"},
	} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	// Enrolling a connection whose only channel is mtls cannot bootstrap
	// itself, and the error has to say which path is left.
	got := xlitest.Run(t, newRoot(root), "connection", "enroll", "lab")
	if !strings.Contains(got.Err.Error(), "--csr") {
		t.Fatal("the mtls-only error does not name the manual path:", got.Err)
	}
	// A check on an expired certificate names the command that renews it.
	authority := signer(t)
	expired(t, root, "work", authority, "10.0.0.9:7349")
	got = xlitest.Run(t, newRoot(root), "connection", "check", "work")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "connection enroll") {
		t.Fatal(got.Err)
	}
}

func signer(t *testing.T) *pki.Authority {
	t.Helper()
	a, err := pki.EnsureCA(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func enrollByHand(t *testing.T, state, name string, a *pki.Authority, address string) {
	t.Helper()
	csr, err := enroll.Request(state, name)
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := a.SignClient(csr, name, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = enroll.Complete(state, name, cert, a.CertificatePEM(), address); err != nil {
		t.Fatal(err)
	}
}

func expired(t *testing.T, state, name string, a *pki.Authority, address string) {
	t.Helper()
	csr, err := enroll.Request(state, name)
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := a.SignClient(csr, name, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = enroll.Complete(state, name, cert, a.CertificatePEM(), address); err != nil {
		t.Fatal(err)
	}
}
