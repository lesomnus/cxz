package webui

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

func (f *browserFixture) Models(_ context.Context, _ *resource.SessionModelsRequest) (*resource.SessionModelsReply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest *resource.SessionEvent
	for _, e := range f.events {
		if e.GetKind() == "models" {
			latest = e
		}
	}
	out := resource.SessionModelsReply_builder{}.Build()
	if latest != nil {
		out.SetData(latest.GetPayload())
		out.SetRunId(latest.GetRunId())
		out.SetCatalogSeq(latest.GetSeq())
		out.SetCatalogMs(latest.GetTimeMs())
	}
	return out, nil
}
