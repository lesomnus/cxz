// Package telemetry delivers numeric usage independently of conversation storage.
package telemetry

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/telemetryauth"
	"github.com/lesomnus/cxz/internal/usagemetrics"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const MaxExportBytes = 4 << 20

func connection(socket string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///cxz-telemetry", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(MaxExportBytes)))
}
func attribute(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

// EventMetrics receives committed events only. No prompt, response, credential,
// provider transcript or tool output is serialized into an export.
func EventMetrics(identity telemetryauth.Identity, event core.Event, v usagemetrics.Values) *collectorpb.ExportMetricsServiceRequest {
	values := map[string]float64{
		"cxz.conversation.events": float64(v.Events), "cxz.conversation.inputs": float64(v.InputMessages),
		"cxz.conversation.assistants": float64(v.AssistantEvents), "cxz.conversation.bytes": float64(v.ConversationBytes),
		"cxz.conversation.event_bytes": float64(v.EventContentBytes), "cxz.turn.finished": float64(v.FinishedTurns),
		"cxz.turn.token_reported": float64(v.TokenReportedTurns), "cxz.turn.cost_reported": float64(v.CostReportedTurns),
	}
	if identity.UsageSource == "supervisor" {
		values["cxz.usage.input_tokens"] = float64(v.InputTokens)
		values["cxz.usage.output_tokens"] = float64(v.OutputTokens)
		values["cxz.usage.cache_read_tokens"] = float64(v.CacheReadTokens)
		values["cxz.usage.cache_write_tokens"] = float64(v.CacheWriteTokens)
		values["cxz.usage.cost_usd"] = v.ReportedCostUSD
	}
	attrs := []*commonpb.KeyValue{attribute("cxz.event.seq", strconv.FormatUint(event.Seq, 10))}
	if event.Response != nil {
		attrs = append(attrs, attribute("model", event.Response.Model), attribute("effort", event.Response.Effort))
	}
	var metrics []*metricspb.Metric
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		value := values[name]
		if value == 0 {
			continue
		}
		metrics = append(metrics, &metricspb.Metric{Name: name, Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
			AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, IsMonotonic: true,
			DataPoints: []*metricspb.NumberDataPoint{{Attributes: attrs, TimeUnixNano: uint64(time.UnixMilli(event.TimeMS).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}},
		}}})
	}
	if len(metrics) == 0 {
		return nil
	}
	var resource []*commonpb.KeyValue
	attributes := identity.Attributes()
	names = names[:0]
	for k := range attributes {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		value := attributes[k]
		resource = append(resource, attribute(k, value))
	}
	return &collectorpb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: resource}, ScopeMetrics: []*metricspb.ScopeMetrics{{Scope: &commonpb.InstrumentationScope{Name: "cxz.supervisor", Version: "1"}, Metrics: metrics}}}}}
}

func exportError(err error, response *collectorpb.ExportMetricsServiceResponse) error {
	if err != nil {
		return err
	}
	if response.GetPartialSuccess().GetRejectedDataPoints() > 0 {
		return fmt.Errorf("telemetry export partially rejected")
	}
	return nil
}
