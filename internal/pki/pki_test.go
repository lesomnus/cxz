package pki

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"
)

func authority(t *testing.T) (*Authority, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pki")
	a, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	return a, dir
}

func enroll(t *testing.T, a *Authority, label string, life time.Duration) (certPEM, keyPEM []byte, record Record) {
	t.Helper()
	keyPEM, csrPEM, err := NewRequest()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, record, err = a.SignClient(csrPEM, label, life)
	if err != nil {
		t.Fatal(err)
	}
	return certPEM, keyPEM, record
}

// The root is the one thing that must survive everything, because every client
// certificate and every pin chains to it. A manager replacement re-runs this.
func TestRootIsCreatedOnceAndNeverReplaced(t *testing.T) {
	a, dir := authority(t)
	first := a.CertificatePEM()
	again, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.CertificatePEM()) != string(first) {
		t.Fatal("the root was replaced on the second call")
	}
	if again.Fingerprint() != a.Fingerprint() || !strings.HasPrefix(a.Fingerprint(), "SHA256:") {
		t.Fatal("fingerprint changed or is unreadable:", a.Fingerprint())
	}
	info, err := os.Stat(filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("the root key is readable:", info.Mode().Perm())
	}
}

// A handshake has to work end to end, by address, with no name anywhere: that
// is the whole point of this CA. The relay accepts the client, the client
// accepts the relay, and neither one consulted a hostname.
func TestEnrolledClientReachesTheRelayByAddress(t *testing.T) {
	a, dir := authority(t)
	if err := a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	server, err := ServerCredentials(ServerDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _ := enroll(t, a, "laptop", 0)
	client, err := ClientCredentials(certPEM, keyPEM, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if peer := handshake(t, server, client); peer != nil {
		t.Fatal("a valid client was refused:", peer)
	}

	// Another installation's root signs certificates this one must not accept.
	other, _ := authority(t)
	foreignCert, foreignKey, _ := enroll(t, other, "intruder", 0)
	foreign, err := ClientCredentials(foreignCert, foreignKey, other.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, server, foreign) == nil {
		t.Fatal("a certificate from another installation was accepted")
	}

	// And a client pinning another root must refuse this relay.
	pinnedElsewhere, err := ClientCredentials(certPEM, keyPEM, other.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, server, pinnedElsewhere) == nil {
		t.Fatal("a client accepted a relay its pinned root did not sign")
	}
}

// The usages are what keep one credential from playing the other's part: a
// client certificate presented as the relay is the attack this prevents.
func TestClientCertificateCannotServeAsTheRelay(t *testing.T) {
	a, dir := authority(t)
	if err := a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _ := enroll(t, a, "laptop", 0)
	impostorDir := t.TempDir()
	os.WriteFile(filepath.Join(impostorDir, "tls.crt"), certPEM, 0600)
	os.WriteFile(filepath.Join(impostorDir, "tls.key"), keyPEM, 0600)
	os.WriteFile(filepath.Join(impostorDir, "ca.crt"), a.CertificatePEM(), 0600)
	impostor, err := ServerCredentials(impostorDir)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ClientCredentials(certPEM, keyPEM, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, impostor, client) == nil {
		t.Fatal("a client certificate was accepted as the relay")
	}

	// The relay's own certificate is equally useless as a client.
	serverCert, _ := os.ReadFile(filepath.Join(ServerDir(dir), "tls.crt"))
	serverKey, _ := os.ReadFile(filepath.Join(ServerDir(dir), "tls.key"))
	asClient, err := ClientCredentials(serverCert, serverKey, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	real, err := ServerCredentials(ServerDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, real, asClient) == nil {
		t.Fatal("the relay's certificate was accepted as a client")
	}
}

// Revoking is the answer to a lost laptop, and it has to apply to a relay that
// is already running: nobody reaches for this while planning a restart.
func TestRevocationAppliesWithoutARestart(t *testing.T) {
	a, dir := authority(t)
	if err := a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, record := enroll(t, a, "lost-laptop", 0)
	server, err := ServerCredentials(ServerDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	client, err := ClientCredentials(certPEM, keyPEM, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, server, client) != nil {
		t.Fatal("the client was refused before revocation")
	}
	if err = a.Revoke(record.Serial); err != nil {
		t.Fatal(err)
	}
	// Same credentials object: the list is read per handshake, not at startup.
	if handshake(t, server, client) == nil {
		t.Fatal("a revoked certificate was still accepted")
	}
	if err = a.Revoke(record.Serial); err != nil {
		t.Fatal("revoking twice is not an error:", err)
	}
	if err = a.Revoke("999"); err == nil {
		t.Fatal("revoked a serial this installation never issued")
	}
	// Another client is unaffected by the first one's revocation.
	otherCert, otherKey, _ := enroll(t, a, "desktop", 0)
	other, err := ClientCredentials(otherCert, otherKey, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, server, other) != nil {
		t.Fatal("revoking one certificate locked out another")
	}
}

func TestExpiredClientCertificateIsRefused(t *testing.T) {
	a, dir := authority(t)
	if err := a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	// The shortest life the signer accepts: valid for a nanosecond, which has
	// passed by the time the handshake looks at it.
	certPEM, keyPEM, _ := enroll(t, a, "stale", time.Nanosecond)
	expiry, serial, err := Expiry(certPEM)
	if err != nil || serial == "" {
		t.Fatal(err, serial)
	}
	if expiry.After(time.Now()) {
		t.Fatal("fixture did not produce an expired certificate:", expiry)
	}
	server, err := ServerCredentials(ServerDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	client, err := ClientCredentials(certPEM, keyPEM, a.CertificatePEM())
	if err != nil {
		t.Fatal(err)
	}
	if handshake(t, server, client) == nil {
		t.Fatal("an expired certificate was accepted")
	}
}

// The relay's leaf is derived state: present and valid is all that matters, and
// refreshing it must not disturb the root that clients pinned.
func TestServerMaterialIsReusedThenReissued(t *testing.T) {
	a, dir := authority(t)
	if err := a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(ServerDir(dir), "tls.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if err = a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(ServerDir(dir), "tls.crt"))
	if string(again) != string(first) {
		t.Fatal("a valid relay certificate was replaced for no reason")
	}
	if err = os.Remove(filepath.Join(ServerDir(dir), "tls.crt")); err != nil {
		t.Fatal(err)
	}
	if err = a.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	third, _ := os.ReadFile(filepath.Join(ServerDir(dir), "tls.crt"))
	if string(third) == string(first) || len(third) == 0 {
		t.Fatal("a missing relay certificate was not reissued")
	}
	ca, _ := os.ReadFile(filepath.Join(ServerDir(dir), "ca.crt"))
	if string(ca) != string(a.CertificatePEM()) {
		t.Fatal("the relay is verifying clients against something other than the root")
	}
	if _, err = os.Stat(filepath.Join(ServerDir(dir), "ca.key")); err == nil {
		t.Fatal("the root's private key was copied into the relay's directory")
	}
}

func TestSignRefusesWhatItCannotName(t *testing.T) {
	a, _ := authority(t)
	_, csrPEM, err := NewRequest()
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"", strings.Repeat("a", 65), "has space", "semi;colon", "new\nline"} {
		if _, _, err = a.SignClient(csrPEM, label, 0); err == nil {
			t.Fatalf("accepted label %q", label)
		}
	}
	if _, _, err = a.SignClient([]byte("not pem"), "laptop", 0); err == nil {
		t.Fatal("accepted a request that is not PEM")
	}
	// A certificate is not a request, however well formed it is.
	if _, _, err = a.SignClient(a.CertificatePEM(), "laptop", 0); err == nil {
		t.Fatal("accepted a certificate in place of a request")
	}
	records, err := a.Issued()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatal("a refused request was recorded as issued:", records)
	}
	enroll(t, a, "laptop", 0)
	if records, _ = a.Issued(); len(records) != 1 || records[0].Label != "laptop" {
		t.Fatal("issuing was not recorded:", records)
	}
}

// handshake reports the first error either side raised, or nil when both
// accepted the other. Both sides are checked because TLS 1.3 lets a client
// finish before the server has judged its certificate: a rejected client often
// learns of it only from the server's alert.
func handshake(t *testing.T, server, client credentials.TransportCredentials) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			accepted <- err
			return
		}
		defer c.Close()
		_, _, err = server.ServerHandshake(c)
		accepted <- err
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, clientErr := client.ClientHandshake(ctx, ln.Addr().String(), c)
	serverErr := <-accepted
	if clientErr != nil {
		return clientErr
	}
	return serverErr
}
