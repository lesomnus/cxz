package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type historyMetricsClient struct{ api.SessionsClient }

func (historyMetricsClient) History(context.Context, *api.WatchRequest, ...grpc.CallOption) (*api.EventBatch, error) {
	return &api.EventBatch{Events: []*api.Event{{Seq: 1, Kind: "assistant", Text: "PRIVATE CONTENT"}}}, nil
}

func TestHistoryRecordingMeasuresPayloadAndFirstRenderWithoutContent(t *testing.T) {
	m := conversationModel()
	m.client = historyMetricsClient{}
	m.debugRecorder = &debugRecorder{}
	m.debugRecorder.Start()
	started := time.Now()
	page, err := m.fetchHistory(context.Background(), "s", 0, "initial")
	if err != nil {
		t.Fatal(err)
	}
	prepared := m.prepareHistoryPage(context.Background(), historyPage{id: "s", initial: true, events: page.Events, started: started}, "claude", m.view.Width)
	m.applyHistoryPage(prepared)
	archive := m.debugRecorder.Stop()
	var rpc, render, preparation bool
	for _, event := range archive.Events {
		if event.Kind == "history_rpc" {
			rpc = event.Count == 1 && event.Bytes == proto.Size(page) && event.Type == "initial"
		}
		if event.Kind == "history_first_render" {
			render = event.Duration > 0
		}
		if event.Kind == "history_prepare" {
			preparation = event.Count == 1
		}
	}
	data, _ := json.Marshal(archive.Events)
	if !rpc || !render || !preparation || strings.Contains(string(data), "PRIVATE") {
		t.Fatal(string(data))
	}
}
