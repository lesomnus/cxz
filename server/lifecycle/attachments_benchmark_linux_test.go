package lifecycle

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type benchmarkUploadRuntime struct {
	*fixture
	root  string
	store bool
}

func (f *benchmarkUploadRuntime) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	if f.store {
		return assets.Add(ctx, f.root, f.p.Id, in.SessionID, in.Name, in.Size, src)
	}
	n, err := io.Copy(io.Discard, src)
	if err == nil && n != in.Size {
		err = fmt.Errorf("size changed")
	}
	return "/cxz/assets/session/file.bin", err
}

type repeatedBody struct {
	remaining int64
	position  int
}

var uploadPattern = make([]byte, 32<<10)

func (r *repeatedBody) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	p = p[:min(int64(len(p)), r.remaining)]
	count := len(p)
	for len(p) > 0 {
		n := copy(p, uploadPattern[r.position:])
		r.position = (r.position + n) % len(uploadPattern)
		p = p[n:]
	}
	r.remaining -= int64(count)
	return count, nil
}

// Same resource lifecycle, loopback network and bounded source on both paths.
// "discard" isolates framing/copying; "storage" includes SHA-256, fsync, CAS
// deduplication and immutable publication. Every case owns/cleans its directory.
func BenchmarkAttachmentDataPath(b *testing.B) {
	for _, store := range []bool{false, true} {
		mode := "discard"
		if store {
			mode = "storage"
		}
		for _, size := range []int64{32 << 10, 8 << 20, 64 << 20} {
			label := fmt.Sprintf("%dMiB", size>>20)
			if size < 1<<20 {
				label = fmt.Sprintf("%dKiB", size>>10)
			}
			for _, workers := range []int{1, 4} {
				for _, wire := range []string{"grpc", "http", "grpc-tls", "http-tls"} {
					b.Run(fmt.Sprintf("%s/%s/workers%d/%s", mode, label, workers, wire), func(b *testing.B) {
						ctx := b.Context()
						root := b.TempDir()
						db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(root, "db"), MaxOpenConns: 1}).Open(ctx)
						if err != nil {
							b.Fatal(err)
						}
						defer db.Close()
						f := &benchmarkUploadRuntime{fixture: &fixture{p: &api.Project{Id: "project", Workspace: "/workspace", State: "running"}, s: &api.Session{Id: "0123456789abcdef01234567", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", RunId: "run", State: "idle"}}, root: filepath.Join(root, "assets"), store: store}
						f.s.AuthBackend = accounts.ProjectLocalOAuth
						f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
						stack, err := Build(ctx, db, f, nil)
						if err != nil {
							b.Fatal(err)
						}
						layer, _ := resource.Find[Layer](stack)
						secure := strings.HasSuffix(wire, "-tls")
						var rawServer *httptest.Server
						if secure {
							rawServer = httptest.NewTLSServer(attachmenthttp.Handler{Uploader: layer})
						} else {
							rawServer = httptest.NewServer(attachmenthttp.Handler{Uploader: layer})
						}
						defer rawServer.Close()
						var options []grpc.ServerOption
						var creds credentials.TransportCredentials = insecure.NewCredentials()
						if secure {
							options = append(options, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: rawServer.TLS.Certificates})))
							pool := x509.NewCertPool()
							pool.AddCert(rawServer.Certificate())
							creds = credentials.NewTLS(&tls.Config{RootCAs: pool})
						}
						g := grpc.NewServer(options...)
						resource.RegisterServer(g, stack)
						defer g.Stop()
						listener, err := net.Listen("tcp", "127.0.0.1:0")
						if err != nil {
							b.Fatal(err)
						}
						go g.Serve(listener)
						conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(creds))
						if err != nil {
							b.Fatal(err)
						}
						defer conn.Close()
						client := resourceclient.New(conn)
						if strings.HasPrefix(wire, "http") {
							client = resourceclient.New(conn, &attachmenthttp.Client{Origin: rawServer.URL, HTTP: rawServer.Client()})
						}
						defer client.Close()
						in := assets.Upload{SessionID: f.s.Id, RunID: "run", Name: "file.bin", Size: size}
						upload := func() {
							if _, err := client.UploadAttachment(ctx, in, &repeatedBody{remaining: size}); err != nil {
								b.Error(err)
							}
						}
						upload()
						runtime.GC()
						var before, after runtime.MemStats
						runtime.ReadMemStats(&before)
						var usageBefore, usageAfter unix.Rusage
						_ = unix.Getrusage(unix.RUSAGE_SELF, &usageBefore)
						var peak atomic.Uint64
						peak.Store(before.HeapAlloc)
						stop, done := make(chan struct{}), make(chan struct{})
						go func() {
							defer close(done)
							ticker := time.NewTicker(2 * time.Millisecond)
							defer ticker.Stop()
							for {
								select {
								case <-stop:
									return
								case <-ticker.C:
									var s runtime.MemStats
									runtime.ReadMemStats(&s)
									for old := peak.Load(); s.HeapAlloc > old; old = peak.Load() {
										if peak.CompareAndSwap(old, s.HeapAlloc) {
											break
										}
									}
								}
							}
						}()
						b.SetBytes(size)
						b.ReportAllocs()
						b.ResetTimer()
						if workers == 1 {
							for i := 0; i < b.N; i++ {
								upload()
							}
						} else {
							b.SetParallelism(max(1, workers/runtime.GOMAXPROCS(0)))
							b.RunParallel(func(pb *testing.PB) {
								for pb.Next() {
									upload()
								}
							})
						}
						b.StopTimer()
						close(stop)
						<-done
						_ = unix.Getrusage(unix.RUSAGE_SELF, &usageAfter)
						runtime.ReadMemStats(&after)
						cpu := usageAfter.Utime.Nano() + usageAfter.Stime.Nano() - usageBefore.Utime.Nano() - usageBefore.Stime.Nano()
						b.ReportMetric(float64(cpu)/float64(b.N)/1000, "cpu-us/op")
						b.ReportMetric(float64(after.NumGC-before.NumGC)/float64(b.N), "GC/op")
						maximum := max(peak.Load(), after.HeapAlloc)
						b.ReportMetric(float64(maximum-before.HeapAlloc)/(1<<20), "heap-growth-MiB")
					})
				}
			}
		}
	}
}
