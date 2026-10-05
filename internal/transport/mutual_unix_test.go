//go:build !windows

package transport

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/pki"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// The relay forwards the manager's unix socket, which only exists where the
// manager does. A Windows client dials the relay; it never runs one.

// relayFixture starts the real relay over a real socket: a manager-shaped
// upstream on root/run/daemon.sock, the authority that signs both sides, and
// the listener a client dials by address.
func relayFixture(t *testing.T) (*pki.Authority, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	backend := grpc.NewServer()
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(backend, h)
	ln, err := net.Listen("unix", Socket(root))
	if err != nil {
		t.Fatal(err)
	}
	go backend.Serve(ln)
	t.Cleanup(backend.Stop)

	authority, err := pki.EnsureCA(filepath.Join(root, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	if err = authority.EnsureServer(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	served := make(chan error, 1)
	go func() { served <- ExposeMutual(ctx, root, address, pki.ServerDir(filepath.Join(root, "pki")), nil) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-served:
			if err != nil {
				t.Log("relay exit:", err)
			}
		case <-time.After(5 * time.Second):
			t.Log("relay did not exit")
		}
	})
	for i := 0; ; i++ {
		c, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			c.Close()
			break
		}
		if i > 100 {
			t.Fatal("relay did not listen:", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return authority, address
}

func identity(t *testing.T, a *pki.Authority, label string, life time.Duration) (Identity, pki.Record) {
	t.Helper()
	keyPEM, csrPEM, err := pki.NewRequest()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, record, err := a.SignClient(csrPEM, label, life)
	if err != nil {
		t.Fatal(err)
	}
	return Identity{Certificate: certPEM, Key: keyPEM, CA: a.CertificatePEM()}, record
}

// The relay reaches the same surface the plaintext one does -- any method,
// unary and streaming -- while the credential is a certificate rather than a
// token that would be readable on the wire it travels over.
func TestMutualRelayCarriesEveryMethod(t *testing.T) {
	authority, address := relayFixture(t)
	id, _ := identity(t, authority, "laptop", 0)
	conn, err := DialEndpoint("mtls://"+address, "", &id)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client := healthpb.NewHealthClient(conn)
	reply, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || reply.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal(reply, err)
	}
	watch, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if reply, err = watch.Recv(); err != nil || reply.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal(reply, err)
	}
}

// A certificate from elsewhere, and one this installation revoked, are both
// refused at the handshake -- before any method is reached.
func TestMutualRelayRefusesForeignAndRevokedClients(t *testing.T) {
	authority, address := relayFixture(t)
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
	// Pins this installation's root, so the client accepts the relay; what it
	// presents is signed by another one, so the relay must not accept it.
	foreign := Identity{Certificate: foreignCert, Key: foreignKey, CA: authority.CertificatePEM()}

	revokedID, record := identity(t, authority, "lost", 0)
	if err = authority.Revoke(record.Serial); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]Identity{"foreign": foreign, "revoked": revokedID} {
		conn, err := DialEndpoint("mtls://"+address, "", &id)
		if err != nil {
			t.Fatal(name, err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		cancel()
		conn.Close()
		if err == nil {
			t.Fatal(name + " client reached the manager")
		}
		if code := status.Code(err); code != codes.Unavailable {
			t.Fatalf("%s: expected a refused connection, got %v: %v", name, code, err)
		}
	}
	// The installation's own client still works, so the refusals above are about
	// those certificates rather than a relay that stopped accepting anything.
	good, _ := identity(t, authority, "laptop", 0)
	conn, err := DialEndpoint("mtls://"+address, "", &good)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
}
