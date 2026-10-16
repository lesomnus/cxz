//go:build !windows

package transport

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"

	"github.com/lesomnus/cxz/internal/pki"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// The relay forwards the manager's unix socket, which only exists where the
// manager does. A Windows client dials the relay; it never runs one.

// uploadRecorder is the manager's attachment store, as far as a relay test
// needs one: what arrived, and what was declared about it.
type uploadRecorder struct {
	mu      sync.Mutex
	last    assets.Upload
	content string
	calls   int
}

func (r *uploadRecorder) UploadAttachment(_ context.Context, in assets.Upload, body io.Reader) (string, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last, r.content, r.calls = in, string(content), r.calls+1
	return "/cxz/assets/" + in.SessionID + "/upload-1/" + in.Name, nil
}

func (r *uploadRecorder) seen() (assets.Upload, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last, r.content, r.calls
}

// relayFixture starts the real relay over a real socket: a manager-shaped
// upstream on root/run/daemon.sock, its HTTP socket beside it so file bodies
// have somewhere to be forwarded to, the authority that signs both sides, and
// the listener a client dials by address.
func relayFixture(t *testing.T) (*pki.Authority, string, *uploadRecorder) {
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
	uploads := &uploadRecorder{}
	stopHTTP, err := StartConnect(t.Context(), root, backend, attachmenthttp.Handler{Uploader: uploads})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopHTTP)

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
	return authority, address, uploads
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
	authority, address, _ := relayFixture(t)
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
	authority, address, _ := relayFixture(t)
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
