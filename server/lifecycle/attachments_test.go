package lifecycle

import (
	"bytes"
	"context"
	"io"
	"net"
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
	"google.golang.org/grpc/test/bufconn"
)

type uploadRuntime struct {
	*fixture
	root     string
	finished chan error
	started  chan struct{}
}

func (f *uploadRuntime) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	if in.SessionID != f.s.Id || in.RunID != f.s.RunId {
		return "", status.Error(codes.FailedPrecondition, "wrong session or run")
	}
	if in.Name == "partial.txt" {
		src = &observedUploadReader{Reader: src, started: f.started}
	}
	path, err := assets.Add(ctx, f.root, f.p.Id, in.SessionID, in.Name, in.Size, src)
	f.finished <- err
	return path, err
}

type observedUploadReader struct {
	io.Reader
	started chan struct{}
}

func (r *observedUploadReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if r.started != nil {
		close(r.started)
		r.started = nil
	}
	return n, err
}

// What storage promises about a file body, over the one path that carries one:
// bytes and name preserved at a size no unary message would hold, a second file
// of the same name not overwriting the first, a declared size that disagrees
// with the stream refused, a stale run refused, and a transfer cancelled with
// bytes in flight leaving nothing published.
func TestFileUpload(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &uploadRuntime{fixture: &fixture{
		p: &api.Project{Id: "project", Workspace: "/workspace", State: "running"},
		s: &api.Session{Id: "0123456789abcdef01234567", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", RunId: "run", State: "idle"},
	}, root: t.TempDir(), finished: make(chan error, 10), started: make(chan struct{})}
	f.s.AuthBackend = accounts.ProjectLocalOAuth
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	httpServer := httptest.NewServer(attachmenthttp.Handler{Uploader: layer})
	defer httpServer.Close()
	ln := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	resource.RegisterServer(server, stack)
	go server.Serve(ln)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///upload", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := resourceclient.New(conn, &attachmenthttp.Client{Origin: httpServer.URL, HTTP: httpServer.Client()})
	defer client.Close()
	wait := func(wantError bool) {
		t.Helper()
		select {
		case err := <-f.finished:
			if (err != nil) != wantError {
				t.Fatalf("server upload result: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("server did not finish upload")
		}
	}
	// Larger than the default unary gRPC limit, with distinct content per frame.
	body := make([]byte, 5*1024*1024+17)
	for i := range body {
		body[i] = byte(i*7 + i/(256*1024))
	}
	request := assets.Upload{SessionID: f.s.Id, RunID: f.s.RunId, Name: "한글 report.txt", Size: int64(len(body))}
	path, err := client.UploadAttachment(ctx, request, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	wait(false)
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, body) || filepath.Base(path) != request.Name {
		t.Fatal("upload changed content or name", err)
	}
	request.Size = 0
	empty, err := client.UploadAttachment(ctx, request, bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	wait(false)
	if empty == path {
		t.Fatal("same filename overwrote the previous upload")
	}

	// Cancelling with bytes in flight must return from flob without an export.
	partial, abort := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := client.UploadAttachment(partial, assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "partial.txt", Size: 6}, &slowReader{content: []byte("abc"), stop: partial.Done()})
		done <- err
	}()
	select {
	case <-f.started:
	case <-ctx.Done():
		t.Fatal("upload did not start")
	}
	abort()
	if err := <-done; err == nil {
		t.Fatal("cancellation lost")
	}
	wait(true)
	entries, err := os.ReadDir(filepath.Join(assets.ExportRoot(f.root, "project"), f.s.Id))
	if err != nil || len(entries) != 2 {
		t.Fatal("partial attachment exposed", len(entries), err)
	}

	for _, tc := range []struct {
		name  string
		input assets.Upload
		body  []byte
	}{
		{"short", assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "short.txt", Size: 4}, []byte("abc")},
		{"long", assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "long.txt", Size: 2}, []byte("abc")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := client.UploadAttachment(ctx, tc.input, bytes.NewReader(tc.body)); err == nil {
				t.Fatal("invalid size accepted")
			}
			wait(true)
		})
	}
	request.RunID = "stale"
	if _, err := client.UploadAttachment(ctx, request, bytes.NewReader(nil)); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("server rejection lost", err)
	}
}

// slowReader hands over its content and then waits, so a transfer can be
// cancelled while the server is reading it rather than after. It waits on the
// cancellation rather than forever: a Read already in progress when the context
// is cancelled is not re-entered, so a reader that never returns holds the
// request open and the test closes nothing.
type slowReader struct {
	content []byte
	sent    bool
	stop    <-chan struct{}
}

func (r *slowReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.content), nil
	}
	<-r.stop
	return 0, io.EOF
}
