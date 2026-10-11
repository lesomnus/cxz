package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

// Every gateway fixture uses the real Manager Connect listener and HTTP proxy,
// rather than another set of web-specific service implementations.
func testManagerRPC(t *testing.T, g *grpc.Server) http.Handler {
	t.Helper()
	root, err := os.MkdirTemp("", "cxz-web-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	stop, err := transport.StartConnect(t.Context(), root, g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	handler, closeProxy := ManagerProxy(root)
	t.Cleanup(closeProxy)
	return handler
}

func TestUnavailableManagerConnectReportsUpgrade(t *testing.T) {
	proxy, closeProxy := ManagerProxy(t.TempDir())
	defer closeProxy()
	server := httptest.NewServer(proxy)
	defer server.Close()
	client := connect.NewClient[resource.SessionListRequest, resource.SessionListResponse](server.Client(), server.URL+"/cxz.SessionService/List")
	_, err := client.CallUnary(t.Context(), connect.NewRequest(&resource.SessionListRequest{}))
	if connect.CodeOf(err) != connect.CodeUnavailable || !strings.Contains(err.Error(), "update and restart the Manager") {
		t.Fatal("upgrade error lost", err)
	}
}
