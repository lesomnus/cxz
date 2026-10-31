package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/usagemetrics"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type capture struct {
	collectorpb.UnimplementedMetricsServiceServer
	calls chan *collectorpb.ExportMetricsServiceRequest
	token string
}

func (c *capture) Export(ctx context.Context, r *collectorpb.ExportMetricsServiceRequest) (*collectorpb.ExportMetricsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer "+c.token {
		panic("missing ingestion authorization")
	}
	c.calls <- r
	return &collectorpb.ExportMetricsServiceResponse{}, nil
}
func testIdentity() telemetryauth.Identity {
	return telemetryauth.Identity{Project: "00112233445566778899aabbccddeeff", Session: "session", Account: "work", Agent: "codex", Run: "run", Source: "supervisor", UsageSource: "supervisor", Scope: "conversation"}
}

func TestRelayProtobufJSONAndPublisher(t *testing.T) {
	i := testIdentity()
	secret := "test-capability"
	token, _ := telemetryauth.Sign(secret, i)
	socket := filepath.Join(t.TempDir(), "otel.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	capture := &capture{calls: make(chan *collectorpb.ExportMetricsServiceRequest, 10), token: token}
	srv := grpc.NewServer()
	collectorpb.RegisterMetricsServiceServer(srv, capture)
	go srv.Serve(ln)
	defer srv.Stop()
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	port.Close()
	relay, err := StartRelay(t.Context(), address, socket, i.Project, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	e := core.Event{Seq: 2, TimeMS: time.Now().UnixMilli(), Kind: "turn_end", RunID: i.Run, Text: "NEVER EXPORT PROMPT", Payload: json.RawMessage(`{"usage":{"inputTokens":100,"cachedInputTokens":30,"outputTokens":10}}`)}
	state := usagemetrics.State{}
	batch := EventMetrics(i, e, state.Event(e))
	for _, kind := range []string{"application/x-protobuf", "application/json"} {
		var b []byte
		if kind == "application/json" {
			b, err = protojson.Marshal(batch)
		} else {
			b, err = proto.Marshal(batch)
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(e.Text)) {
			t.Fatal("conversation leaked into metrics")
		}
		r, err := http.NewRequest("POST", "http://"+address+"/v1/metrics", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", kind)
		r.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal(res.Status, string(body))
		}
		select {
		case <-capture.calls:
		case <-time.After(time.Second):
			t.Fatal("relay did not forward")
		}
	}
	r, _ := http.NewRequest("POST", "http://"+address+"/v1/metrics", strings.NewReader("invalid"))
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal(res.Status)
	}
	p, err := NewPublisher(socket, secret, i)
	if err != nil {
		t.Fatal(err)
	}
	p.Observe(e)
	p.Close()
	select {
	case got := <-capture.calls:
		if !proto.Equal(got, batch) {
			t.Fatal("different normalized metrics")
		}
	case <-time.After(time.Second):
		t.Fatal("close lost final metrics")
	}
}

func TestNativeSourceAndConfiguration(t *testing.T) {
	for _, test := range []struct {
		agent, version string
		want           bool
	}{{"codex", "codex-cli 0.164.0", true}, {"codex", "0.154.0", false}, {"claude", "2.1.267", true}, {"claude", "unknown", false}} {
		if got := NativeUsage(test.agent, test.version); got != test.want {
			t.Fatal(test, got)
		}
	}
	_, env := NativeLaunch("claude", RelayEndpoint, "token", nil, []string{"OTEL_METRICS_EXPORTER=console", "KEEP=1"})
	if strings.Count(strings.Join(env, "\n"), "OTEL_METRICS_EXPORTER=") != 1 || !strings.Contains(strings.Join(env, "\n"), "KEEP=1") {
		t.Fatal(env)
	}
	i := testIdentity()
	i.UsageSource = "native"
	b := EventMetrics(i, core.Event{TimeMS: time.Now().UnixMilli()}, usagemetrics.Values{FinishedTurns: 1, InputTokens: 100, ReportedCostUSD: 3})
	for _, r := range b.ResourceMetrics {
		for _, s := range r.ScopeMetrics {
			for _, m := range s.Metrics {
				if strings.HasPrefix(m.Name, "cxz.usage.") {
					t.Fatal("native usage double-counted", m.Name)
				}
			}
		}
	}
}
