package kube_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/lesomnus/cxz/internal/kube"
)

func client(t *testing.T, handler http.Handler) *kube.Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := kube.New(kube.Config{Server: srv.URL, CA: ca, Namespace: "work", Token: "secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestKubeconfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	ca := base64.StdEncoding.EncodeToString([]byte("ca-bytes"))
	if err := os.WriteFile(path, []byte(`apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: other
  cluster: {server: https://wrong}
- name: cluster
  cluster: {server: "https://10.0.0.1:6443", certificate-authority-data: `+ca+`}
users:
- name: me
  user: {token: abc}
contexts:
- name: dev
  context: {cluster: cluster, user: me, namespace: work}
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := kube.Kubeconfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://10.0.0.1:6443" || string(cfg.CA) != "ca-bytes" || cfg.Token != "abc" || cfg.Namespace != "work" {
		t.Fatalf("%+v", cfg)
	}
	if err = os.WriteFile(path, []byte("current-context: dev\ncontexts:\n- name: dev\n  context: {cluster: c, user: u}\nclusters:\n- name: c\n  cluster: {server: https://x}\nusers:\n- name: u\n  user: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = kube.Kubeconfig(path); err == nil {
		t.Fatal("a user without a bearer token was accepted")
	}
}

func TestRequests(t *testing.T) {
	var seen []string
	c := client(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "unauthorized", 401)
			return
		}
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+r.Header.Get("Content-Type"))
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/missing"):
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"kind": "Status", "reason": "NotFound", "message": "pods \"missing\" not found"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"reason": "NotFound", "message": "gone"})
		case r.Method == http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			w.Write(body)
		default:
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]string{{"name": "a"}}})
		}
	}))
	ctx := context.Background()
	var out map[string]any
	if err := c.Apply(ctx, c.Path("pods", "p"), map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]string{"name": "p"}}, &out); err != nil {
		t.Fatal(err)
	}
	if out["kind"] != "Pod" {
		t.Fatal(out)
	}
	err := c.Get(ctx, c.Path("pods", "missing"), nil)
	if !kube.IsNotFound(err) || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	if err = c.Delete(ctx, c.Path("pods", "gone")); err != nil {
		t.Fatal("deleting what is already gone is not an error", err)
	}
	var list struct{ Items []map[string]string }
	if err = c.List(ctx, c.Path("pods"), "cxz.owner=o", &list); err != nil || len(list.Items) != 1 {
		t.Fatal(list, err)
	}
	want := []string{
		"PATCH /api/v1/namespaces/work/pods/p?fieldManager=cxz&force=true application/apply-patch+yaml",
		"GET /api/v1/namespaces/work/pods/missing? ",
		"DELETE /api/v1/namespaces/work/pods/gone? application/json",
		"GET /api/v1/namespaces/work/pods?labelSelector=cxz.owner%3Do ",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(seen, "\n"))
	}
}

// execServer speaks the v5 streaming protocol: it upper-cases stdin onto
// stdout until stdin is closed, writes stderr, and ends with status.
func execServer(t *testing.T, status string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/pods/p/exec") || strings.Join(r.URL.Query()["command"], " ") != "sh -c cat" || r.URL.Query().Get("stdin") != "true" {
			http.Error(w, "unexpected exec "+r.URL.String(), 400)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"v5.channel.k8s.io"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		var in bytes.Buffer
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if msg[0] == 255 && len(msg) == 2 && msg[1] == 0 {
				break
			}
			if msg[0] == 0 {
				in.Write(msg[1:])
			}
		}
		conn.Write(ctx, websocket.MessageBinary, append([]byte{1}, bytes.ToUpper(in.Bytes())...))
		conn.Write(ctx, websocket.MessageBinary, append([]byte{2}, "warning\n"...))
		conn.Write(ctx, websocket.MessageBinary, append([]byte{3}, status...))
		conn.Close(websocket.StatusNormalClosure, "")
	})
}

func TestExec(t *testing.T) {
	failure := `{"status":"Failure","reason":"NonZeroExitCode","message":"exit 3","details":{"causes":[{"reason":"ExitCode","message":"3"}]}}`
	c := client(t, execServer(t, failure))
	var stdout, stderr bytes.Buffer
	err := c.Exec(context.Background(), kube.ExecOptions{Pod: "p", Command: []string{"sh", "-c", "cat"}, Stdin: strings.NewReader("hello\n"), Stdout: &stdout, Stderr: &stderr})
	var exit *kube.ExitError
	if !errors.As(err, &exit) || exit.Code != 3 {
		t.Fatalf("exit status lost: %v", err)
	}
	if stdout.String() != "HELLO\n" || stderr.String() != "warning\n" {
		t.Fatalf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
	c = client(t, execServer(t, `{"status":"Success"}`))
	if err = c.Exec(context.Background(), kube.ExecOptions{Pod: "p", Command: []string{"sh", "-c", "cat"}, Stdin: strings.NewReader("x"), Stdout: io.Discard}); err != nil {
		t.Fatal(err)
	}
}
