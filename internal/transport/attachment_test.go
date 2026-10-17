//go:build !windows

package transport

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestAttachmentClientUsesPrivateManagerSocketAndPreservesRemoteSchemes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	healthpb.RegisterHealthServer(g, health.NewServer())
	defer g.Stop()
	var request assets.Upload
	stop, err := StartConnect(t.Context(), root, g, attachmenthttp.Handler{Uploader: attachmenthttp.UploaderFunc(func(ctx context.Context, in assets.Upload, body io.Reader) (string, error) {
		request = in
		return assets.Add(ctx, filepath.Join(root, "assets"), "project", in.SessionID, in.Name, in.Size, body)
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for _, endpoint := range []Endpoint{{}, {Scheme: "local", Address: root}, {Scheme: "unix", Address: Socket(root)}} {
		client := AttachmentClient(root, endpoint, Credential{})
		defer client.Close()
		if ok, err := client.Supported(t.Context()); err != nil || !ok {
			t.Fatal(ok, err)
		}
		path, err := client.UploadAttachment(t.Context(), assets.Upload{SessionID: "session", RunID: "run", Name: "file.txt", Size: 4}, strings.NewReader("data"))
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "data" || request.RunID != "run" {
			t.Fatal("private raw transport changed bytes or identity", err)
		}
	}
	// A relay is reached as itself, on its own address and with the credential
	// its calls use. Without that credential there is nothing to reach it with,
	// and a socket this client did not place is never guessed at.
	for _, endpoint := range []Endpoint{{Scheme: "tcp", Address: "host:1"}, {Scheme: "mtls", Address: "host:1"}, {Scheme: "unix", Address: "/different/socket"}, {Scheme: "tcp", Address: "no-port"}} {
		if AttachmentClient(root, endpoint, Credential{}) != nil {
			t.Fatal("invented raw remote endpoint", endpoint)
		}
	}
	if got := AttachmentClient(root, Endpoint{Scheme: "tcp", Address: "host:1"}, Credential{Token: "t"}); got == nil || got.Origin != "http://host:1" {
		t.Fatal("plaintext relay origin", got)
	}
	if got := AttachmentClient(root, Endpoint{Scheme: "mtls", Address: "host:1"}, Credential{Identity: &Identity{}}); got != nil {
		t.Fatal("unusable client identity accepted", got)
	}
	stop()
	client := AttachmentClient(root, Endpoint{}, Credential{})
	defer client.Close()
	if ok, err := client.Supported(t.Context()); ok || err != nil {
		t.Fatal("absent HTTP socket broke compatibility negotiation", ok, err)
	}
}
