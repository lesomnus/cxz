package lifecycle

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Use the real lifecycle and resource client, with native RPC Upload forbidden:
// success proves the TUI data path stayed raw while aliases use normal Get.
type noRPCUpload struct {
	resource.SessionServiceServer
	calls int
}

func (s *noRPCUpload) Upload(grpc.ClientStreamingServer[resource.SessionUploadRequest, resource.SessionAttachment]) error {
	s.calls++
	return status.Error(codes.Internal, "unexpected RPC upload")
}

func TestResourceClientRawUploadRetainsLifecycleAndAliasChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &uploadRuntime{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/workspace", State: "running"}, s: &api.Session{Id: "0123456789abcdef01234567", Alias: "oak", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", RunId: "run", State: "idle"}}, root: t.TempDir(), finished: make(chan error, 8)}
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	httpServer := httptest.NewServer(attachmenthttp.Handler{Uploader: layer})
	defer httpServer.Close()
	raw := &attachmenthttp.Client{Origin: httpServer.URL, HTTP: httpServer.Client()}
	g := grpc.NewServer()
	native := &noRPCUpload{SessionServiceServer: stack.Session()}
	resource.RegisterSessionServiceServer(g, native)
	defer g.Stop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go g.Serve(listener)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := resourceclient.New(conn, raw)
	defer client.Close()
	data := bytes.Repeat([]byte{0, 255, 10, 13}, (5<<20)/4)
	v, err := resource.NewSessionServiceClient(conn).Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(f.s.Id), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil || v.GetAlias() == "" {
		t.Fatal("session alias unavailable", err)
	}
	for _, ref := range []string{f.s.Id, v.GetAlias()} {
		in := assets.Upload{SessionID: ref, RunID: "run", Name: "한글 report.bin", Size: int64(len(data))}
		path, err := client.UploadAttachment(ctx, in, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		stored, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(stored, data) {
			t.Fatal("raw bytes changed", err)
		}
		in.RunID = "stale"
		if _, err := client.UploadAttachment(ctx, in, bytes.NewReader(data)); status.Code(err) != codes.FailedPrecondition {
			t.Fatal("stale run accepted", err)
		}
	}
	_, err = client.UploadAttachment(ctx, assets.Upload{SessionID: "ffffffffffffffffffffffff", RunID: "run", Name: "wrong.bin", Size: 4}, bytes.NewReader([]byte("data")))
	if status.Code(err) != codes.NotFound {
		t.Fatal("unknown session accepted", err)
	}
	if native.calls != 0 {
		t.Fatal("raw path invoked protobuf Upload")
	}
	// A missing endpoint can choose RPC before bytes are sent. A failed POST
	// cannot, even when the HTTP connection drops before its receipt arrives.
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	legacy := resourceclient.New(conn, &attachmenthttp.Client{Origin: missing.URL, HTTP: missing.Client()})
	defer legacy.Close()
	_, err = legacy.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file", Size: 4}, bytes.NewReader([]byte("data")))
	if status.Code(err) != codes.Internal || native.calls != 1 {
		t.Fatal("RPC compatibility not selected", err, native.calls)
	}
	dropped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set(attachmenthttp.ProtocolHeader, "1")
			w.WriteHeader(204)
			return
		}
		io.Copy(io.Discard, r.Body)
		c, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			c.Close()
		}
	}))
	defer dropped.Close()
	uncertain := resourceclient.New(conn, &attachmenthttp.Client{Origin: dropped.URL, HTTP: dropped.Client()})
	defer uncertain.Close()
	_, err = uncertain.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file", Size: 4}, bytes.NewReader([]byte("data")))
	if err == nil || native.calls != 1 {
		t.Fatal("uncertain POST replayed through RPC", err, native.calls)
	}
	// A bound database layer rejects runtime effects before it reads a body.
	bound := layer
	bound.bound = true
	if _, err := bound.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id}, io.LimitReader(bytes.NewReader(data), 4)); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("transaction restriction bypassed", err)
	}
}
