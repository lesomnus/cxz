package server

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/lifecycle"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Exercise the production runtime wrapper, which the lifecycle fake bypasses.
func TestAuxiliaryLoginRPCUsesManagerRuntime(t *testing.T) {
	for _, manager := range []bool{true, false} {
		name := "project"
		if manager {
			name = "manager"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s := &Server{}
			if manager {
				// No installation: reaching this Manager's preflight is sufficient
				// to verify dispatch without launching Docker or authenticating.
				s.manager = &workspace.Manager{Root: t.TempDir()}
			}
			stack, err := lifecycle.Build(ctx, db, s)
			if err != nil {
				t.Fatal(err)
			}
			_, err = stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work", Agent: "claude", AuthBackend: accounts.ProjectLocalOAuth}.Build())
			if err != nil {
				t.Fatal(err)
			}
			ln := bufconn.Listen(1 << 20)
			g := grpc.NewServer()
			resource.RegisterServer(g, stack)
			go g.Serve(ln)
			defer g.Stop()
			conn, err := grpc.NewClient("passthrough:///auxiliary", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			err = resourceclient.New(conn).LoginAuxiliary(ctx, "work", io.NopCloser(strings.NewReader("")), io.Discard)
			if manager {
				if status.Code(err) == codes.Unimplemented || !strings.Contains(status.Convert(err).Message(), "auxiliary execution requires an installed Manager") {
					t.Fatalf("login did not reach Manager preflight: %v", err)
				}
			} else if status.Code(err) != codes.FailedPrecondition || !strings.Contains(status.Convert(err).Message(), "manager endpoint") {
				t.Fatalf("project runtime should direct login to Manager: %v", err)
			}
		})
	}
}
