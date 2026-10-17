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

// Use the real lifecycle and resource client. There is no longer an RPC upload
// to forbid -- the method does not exist -- so what this holds is the rest: the
// raw path keeps the lifecycle checks, resolves an alias through a normal Get,
// and reports a failure as a failure rather than finding another way.

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
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	httpServer := httptest.NewServer(attachmenthttp.Handler{Uploader: layer})
	defer httpServer.Close()
	raw := &attachmenthttp.Client{Origin: httpServer.URL, HTTP: httpServer.Client()}
	g := grpc.NewServer()
	resource.RegisterSessionServiceServer(g, stack.Session())
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
	// A Manager that does not serve the endpoint is an error rather than a
	// different path, and an upload that cannot happen must not read as one
	// that did.
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	absent := resourceclient.New(conn, &attachmenthttp.Client{Origin: missing.URL, HTTP: missing.Client()})
	defer absent.Close()
	if path, err := absent.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file", Size: 4}, bytes.NewReader([]byte("data"))); err == nil || path != "" {
		t.Fatal("a missing endpoint reported an upload", path, err)
	}
	// A connection with no attachment endpoint at all says so, rather than
	// reaching for a method that no longer exists.
	none := resourceclient.New(conn)
	defer none.Close()
	if _, err := none.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file", Size: 4}, bytes.NewReader([]byte("data"))); status.Code(err) != codes.Unimplemented {
		t.Fatal("a connection without a data path did not say so", err)
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
	// A receipt lost after the body was accepted is uncertain, and publication
	// may already have happened. It is reported, never retried.
	_, err = uncertain.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file", Size: 4}, bytes.NewReader([]byte("data")))
	if err == nil {
		t.Fatal("a lost receipt read as success")
	}
	// A bound database layer rejects runtime effects before it reads a body.
	bound := layer
	bound.bound = true
	if _, err := bound.UploadAttachment(ctx, assets.Upload{SessionID: f.s.Id}, io.LimitReader(bytes.NewReader(data), 4)); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("transaction restriction bypassed", err)
	}
}
