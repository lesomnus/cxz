package telemetry

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/usagemetrics"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Publisher is owned by one Supervisor. Observe is called under its existing
// event lock, never does I/O and never blocks on telemetry. At most 128 small
// numeric batches are buffered; the Collector owns the bounded durable queue.
type Publisher struct {
	identity telemetryauth.Identity
	state    usagemetrics.State
	queue    chan delivery
	cancel   context.CancelFunc
	done     chan struct{}
	dropped  atomic.Uint64
}
type delivery struct {
	batch   *collectorpb.ExportMetricsServiceRequest
	flushed chan struct{}
}

func NewPublisher(socket, secret string, identity telemetryauth.Identity) (*Publisher, error) {
	token, err := telemetryauth.Sign(secret, identity)
	if err != nil {
		return nil, err
	}
	conn, err := connection(socket)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Publisher{identity: identity, queue: make(chan delivery, 128), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		defer conn.Close()
		client := collectorpb.NewMetricsServiceClient(conn)
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-p.queue:
				if item.flushed != nil {
					close(item.flushed)
					continue
				}
				batch := item.batch
				// Wait for a reconnect briefly. A prolonged outage drops
				// metrics with an observable count, never agent output or work.
				q, done := context.WithTimeout(ctx, 3*time.Second)
				response, err := client.Export(metadata.AppendToOutgoingContext(q, "authorization", "Bearer "+token), batch, grpc.WaitForReady(true))
				done()
				if exportError(err, response) != nil {
					p.dropped.Add(1)
				}
			}
		}
	}()
	return p, nil
}

func (p *Publisher) Observe(e core.Event) {
	if p == nil {
		return
	}
	batch := EventMetrics(p.identity, e, p.state.Event(e))
	if batch == nil {
		return
	}
	select {
	case p.queue <- delivery{batch: batch}:
	default:
		p.dropped.Add(1)
	}
}
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	// Give a final turn at most two seconds to reach the durable Collector.
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	flushed := make(chan struct{})
	select {
	case p.queue <- delivery{flushed: flushed}:
		select {
		case <-flushed:
		case <-deadline.C:
		}
	case <-deadline.C:
	}
	p.cancel()
	<-p.done
	if n := p.dropped.Load(); n > 0 {
		fmt.Fprintf(os.Stderr, "telemetry: %d batches dropped; agent execution was unaffected\n", n)
	}
}
