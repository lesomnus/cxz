package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/observe"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func diagnosticsStack(t *testing.T, o *observe.Log) resource.InstallationServiceServer {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stack, err := Build(ctx, db, &fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", State: "idle", RunId: "run", CreateId: "create", Agent: "claude"},
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	return layerOf(t, stack).Installation()
}

func TestDiagnosticsReportsWhatTheManagerDid(t *testing.T) {
	o := observe.New(t.TempDir())
	t.Cleanup(func() { o.Close() })
	for _, c := range []struct {
		method string
		err    error
	}{
		{"/cxz.SessionService/Get", nil},
		{"/cxz.SessionService/Get", nil},
		{"/cxz.SessionService/Send", status.Error(codes.DeadlineExceeded, "no")},
	} {
		_, _ = o.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: c.method},
			func(ctx context.Context, _ any) (any, error) {
				observe.Waited(ctx, "update gate", 5*time.Millisecond)
				return nil, c.err
			})
	}
	ctx := context.Background()
	stack := diagnosticsStack(t, o)

	out, err := stack.Diagnostics(ctx, resource.DiagnosticsRequest_builder{StatsOnly: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.GetStats()) != 2 || out.GetStats()[0].GetMethod() != "/cxz.SessionService/Get" || out.GetStats()[0].GetCalls() != 2 {
		t.Fatal("aggregates", out.GetStats())
	}
	if len(out.GetRecords()) != 0 {
		t.Fatal("stats_only returned records")
	}
	if out.GetPath() == "" {
		t.Fatal("reply does not say where records are kept")
	}

	out, err = stack.Diagnostics(ctx, resource.DiagnosticsRequest_builder{Method: ptr("/cxz.SessionService/Send")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.GetRecords()) != 1 {
		t.Fatal("method filter", out.GetRecords())
	}
	r := out.GetRecords()[0]
	if r.GetCode() != "DeadlineExceeded" || r.GetWaitedOn() != "update gate" || r.GetWaitedMs() < 5 {
		t.Fatal("record lost what it waited on", r)
	}
}

func TestCaptureDiagnosticsOpensAndClosesAWindow(t *testing.T) {
	o := observe.New(t.TempDir())
	t.Cleanup(func() { o.Close() })
	ctx := context.Background()
	stack := diagnosticsStack(t, o)
	out, err := stack.Capture(ctx, resource.CaptureRequest_builder{Seconds: ptr(int32(60))}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasCapturingUntil() || o.CapturingUntil().IsZero() {
		t.Fatal("window did not open")
	}
	if out, err = stack.Capture(ctx, resource.CaptureRequest_builder{Seconds: ptr(int32(0))}.Build()); err != nil {
		t.Fatal(err)
	}
	if out.HasCapturingUntil() || !o.CapturingUntil().IsZero() {
		t.Fatal("window did not close")
	}
	if _, err = stack.Capture(ctx, resource.CaptureRequest_builder{Seconds: ptr(int32(-1))}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal("negative window accepted", err)
	}
}

// A server that collects nothing says so, rather than answering with an empty
// account of itself.
func TestDiagnosticsWithoutCollectionIsUnavailable(t *testing.T) {
	ctx := context.Background()
	stack := diagnosticsStack(t, nil)
	if _, err := stack.Diagnostics(ctx, resource.DiagnosticsRequest_builder{}.Build()); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := stack.Capture(ctx, resource.CaptureRequest_builder{}.Build()); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}
