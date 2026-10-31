package telemetry

import (
	"context"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/telemetryauth"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const RelayAddress = "127.0.0.1:7347"
const RelayEndpoint = "http://" + RelayAddress + "/v1/metrics"

// StartRelay exposes only OTLP metrics on a project's loopback interface. The
// port stays fixed across runtime restarts so running agents need no restart.
func StartRelay(ctx context.Context, address, socket, project, secret string) (io.Closer, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	conn, err := connection(socket)
	if err != nil {
		ln.Close()
		return nil, err
	}
	client := collectorpb.NewMetricsServiceClient(conn)
	slots := make(chan struct{}, 8)
	srv := &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/metrics" {
			http.NotFound(w, r)
			return
		}
		token, e := telemetryauth.Bearer(r.Header.Get("Authorization"))
		if e != nil {
			http.Error(w, "unauthorized telemetry", http.StatusUnauthorized)
			return
		}
		if _, e = telemetryauth.VerifyProject(project, secret, token); e != nil {
			http.Error(w, "unauthorized telemetry", http.StatusUnauthorized)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxExportBytes))
		if e != nil {
			http.Error(w, "export too large", http.StatusRequestEntityTooLarge)
			return
		}
		kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		batch := new(collectorpb.ExportMetricsServiceRequest)
		switch kind {
		case "application/json":
			e = protojson.Unmarshal(body, batch)
		case "application/x-protobuf":
			e = proto.Unmarshal(body, batch)
		default:
			http.Error(w, "unsupported OTLP encoding", http.StatusUnsupportedMediaType)
			return
		}
		if e != nil {
			http.Error(w, "invalid OTLP metrics", http.StatusBadRequest)
			return
		}
		q, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		result, e := client.Export(metadata.AppendToOutgoingContext(q, "authorization", "Bearer "+token), batch)
		if e != nil {
			http.Error(w, "telemetry collector unavailable", http.StatusServiceUnavailable)
			return
		}
		var output []byte
		if kind == "application/json" {
			output, e = protojson.Marshal(result)
		} else {
			output, e = proto.Marshal(result)
		}
		if e != nil {
			http.Error(w, "encoding failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", kind)
		_, _ = w.Write(output)
	})
	r := &relay{server: srv, conn: conn, done: make(chan struct{})}
	go func() { _ = srv.Serve(ln) }()
	go func() {
		select {
		case <-ctx.Done():
			r.Close()
		case <-r.done:
		}
	}()
	return r, nil
}

type relay struct {
	server *http.Server
	conn   io.Closer
	once   sync.Once
	done   chan struct{}
}

func (r *relay) Close() error {
	r.once.Do(func() { close(r.done); _ = r.conn.Close(); _ = r.server.Close() })
	return nil
}
