//go:build !windows

package transport

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// A relay carries a file body on the address and with the credential its calls
// already use. Nothing about the body is wrapped in protobuf on the way, which
// is the entire reason this exists; what this test holds is that the body
// arrives intact, that the declared metadata arrives with it, and that the
// calls still work beside it on the one listener.
func TestMutualRelayCarriesFileBodiesBesideCalls(t *testing.T) {
	authority, address, uploads := relayFixture(t)
	id, _ := identity(t, authority, "laptop", 0)
	client := AttachmentClient(t.TempDir(), Endpoint{Scheme: "mtls", Address: address}, Credential{Identity: &id})
	if client == nil {
		t.Fatal("no raw client for an enrolled relay")
	}
	defer client.Close()
	if ok, err := client.Supported(t.Context()); err != nil || !ok {
		t.Fatal("relay did not advertise the raw endpoint", ok, err)
	}
	content := "한글 body\r\n\x00\xff"
	path, err := client.UploadAttachment(t.Context(), assets.Upload{SessionID: "session-1", RunID: "run-1", Name: "report.txt", Size: int64(len(content))}, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	seen, got, calls := uploads.seen()
	if path != "/cxz/assets/session-1/upload-1/report.txt" || got != content || calls != 1 {
		t.Fatal("relay changed the body or the path", path, got, calls)
	}
	if seen.RunID != "run-1" || seen.Name != "report.txt" || seen.Size != int64(len(content)) {
		t.Fatal("relay changed the declared upload", seen)
	}
	// The same listener, still answering calls.
	conn, err := DialEndpoint("mtls://"+address, "", &id)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if reply, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil || reply.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal("calls stopped working beside the bodies", reply, err)
	}
}

// A certificate this installation did not sign cannot post a body, for the same
// reason it cannot make a call: the handshake refuses it. The store must not be
// reached at all -- not refused after the bytes arrive.
func TestMutualRelayRefusesForeignBodiesAtTheHandshake(t *testing.T) {
	authority, address, uploads := relayFixture(t)
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
	foreign := Identity{Certificate: foreignCert, Key: foreignKey, CA: authority.CertificatePEM()}
	client := AttachmentClient(t.TempDir(), Endpoint{Scheme: "mtls", Address: address}, Credential{Identity: &foreign})
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := client.UploadAttachment(ctx, assets.Upload{SessionID: "session-1", RunID: "run-1", Name: "report.txt", Size: 4}, strings.NewReader("data")); err == nil {
		t.Fatal("a foreign certificate posted a body")
	}
	if _, _, calls := uploads.seen(); calls != 0 {
		t.Fatal("a refused body reached the store")
	}
}

// The plaintext relay authenticates a body with the token its calls carry, in
// the header its calls carry it in. A request without it is refused before the
// body is read, so a wrong token does not first cost the transfer.
func TestPlaintextRelayCarriesBodiesWithTheCallsToken(t *testing.T) {
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

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	probe.Close()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	served := make(chan error, 1)
	go func() { served <- Expose(ctx, root, "tcp://"+address, "secret", nil) }()
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
		if c, err := net.DialTimeout("tcp", address, time.Second); err == nil {
			c.Close()
			break
		} else if i > 100 {
			t.Fatal("relay did not listen:", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	endpoint := Endpoint{Scheme: "tcp", Address: address}
	client := AttachmentClient(root, endpoint, Credential{Token: "secret"})
	defer client.Close()
	content := "plaintext body"
	path, err := client.UploadAttachment(t.Context(), assets.Upload{SessionID: "session-1", RunID: "run-1", Name: "report.txt", Size: int64(len(content))}, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, got, calls := uploads.seen(); path == "" || got != content || calls != 1 {
		t.Fatal("relay changed the body", path, got, calls)
	}
	// gRPC still reaches the manager through the same port.
	conn, err := DialEndpoint("tcp://"+address, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	check, done := context.WithTimeout(t.Context(), 10*time.Second)
	defer done()
	if reply, err := healthpb.NewHealthClient(conn).Check(check, &healthpb.HealthCheckRequest{}); err != nil || reply.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal("calls stopped working beside the bodies", reply, err)
	}

	wrong := AttachmentClient(root, endpoint, Credential{Token: "guess"})
	defer wrong.Close()
	if ok, err := wrong.Supported(t.Context()); ok || err == nil {
		t.Fatal("discovery accepted a wrong token", ok, err)
	}
	if _, err := wrong.UploadAttachment(t.Context(), assets.Upload{SessionID: "session-1", RunID: "run-1", Name: "report.txt", Size: 1}, badReader{t}); status.Code(err) != codes.Unauthenticated {
		t.Fatal("a wrong token uploaded a body", err)
	}
	if _, _, calls := uploads.seen(); calls != 1 {
		t.Fatal("a refused body reached the store")
	}
}

// badReader fails the test if it is read: a refused upload must be refused
// before its body is consumed.
type badReader struct{ t *testing.T }

func (r badReader) Read([]byte) (int, error) {
	r.t.Error("a refused upload read its source")
	return 0, io.EOF
}
