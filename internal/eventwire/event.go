package eventwire

import (
	"encoding/json"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/resource"
)

func ToResource(e *api.Event) *resource.SessionEvent {
	v := resource.SessionEvent_builder{RunId: e.RunId, Seq: e.Seq, TimeMs: e.TimeMs, Kind: e.Kind, Text: e.Text, RequestId: e.RequestId, Payload: e.Payload}.Build()
	if r := e.Response; r != nil {
		v.SetResponse(resource.ResponseMetadata_builder{Model: r.Model, Effort: r.Effort, ModelSource: r.ModelSource, EffortSource: r.EffortSource, TurnId: r.TurnId, Phase: r.Phase, CompletionJson: r.CompletionJson}.Build())
	}
	if t := e.ToolSummary; t != nil {
		var files []*resource.ToolFileSummary
		for _, f := range t.Files {
			files = append(files, resource.ToolFileSummary_builder{Path: f.Path, MovePath: f.MovePath, Action: f.Action, Added: f.Added, Removed: f.Removed, Lines: f.Lines, Measure: f.Measure, PerMatch: f.PerMatch}.Build())
		}
		v.SetToolSummary(resource.ToolSummary_builder{Name: t.Name, Shell: t.Shell, Command: t.Command, State: t.State, Files: files, OmittedFiles: t.OmittedFiles, Kind: t.Kind, Description: t.Description}.Build())
	}
	return v
}
func FromResource(id string, e *resource.SessionEvent) *api.Event {
	v := &api.Event{SessionId: id, RunId: e.GetRunId(), Seq: e.GetSeq(), TimeMs: e.GetTimeMs(), Kind: e.GetKind(), Text: e.GetText(), RequestId: e.GetRequestId(), Payload: e.GetPayload()}
	if r := e.GetResponse(); r != nil {
		v.Response = &api.ResponseMetadata{Model: r.GetModel(), Effort: r.GetEffort(), ModelSource: r.GetModelSource(), EffortSource: r.GetEffortSource(), TurnId: r.GetTurnId(), Phase: r.GetPhase(), CompletionJson: r.GetCompletionJson()}
	}
	if t := e.GetToolSummary(); t != nil {
		var files []*api.ToolFileSummary
		for _, f := range t.GetFiles() {
			files = append(files, &api.ToolFileSummary{Path: f.GetPath(), MovePath: f.GetMovePath(), Action: f.GetAction(), Added: f.GetAdded(), Removed: f.GetRemoved(), Lines: f.GetLines(), Measure: f.GetMeasure(), PerMatch: f.GetPerMatch()})
		}
		v.ToolSummary = &api.ToolSummary{Name: t.GetName(), Shell: t.GetShell(), Command: t.GetCommand(), State: t.GetState(), Files: files, OmittedFiles: t.GetOmittedFiles(), Kind: t.GetKind(), Description: t.GetDescription()}
	}
	return v
}

// FromCore is a journal record as a reader sees it. Two kinds are rewritten
// rather than passed through, which is why this is a conversion and not a field
// copy: a history checkpoint becomes the boundary a client can show, and a raw
// background frame becomes the background event derived from it. It lives here
// so that the store a session writes and the server that reads it apply the
// same rules -- there is one view of a record, not one per caller.
func FromCore(e core.Event) *api.Event {
	if e.Kind == core.HistoryCheckpointKind {
		e.Kind = core.HistoryTrimmedKind
		e.Text = "Earlier display history was removed by the size limit."
		e.Payload, _ = json.Marshal(core.HistoryBoundary{Through: e.Seq})
	}

	if e.Kind == "raw" && agentview.IsBackgroundEvent(e.Raw) {
		e.Kind, e.Payload = "background", e.Raw
	}
	v := &api.Event{SessionId: e.SessionID, RunId: e.RunID, Seq: e.Seq, TimeMs: e.TimeMS, Kind: e.Kind, Text: e.Text, RequestId: e.RequestID, Payload: e.Payload}
	if r := e.Response; r != nil {
		v.Response = &api.ResponseMetadata{Model: r.Model, Effort: r.Effort, ModelSource: r.ModelSource, EffortSource: r.EffortSource, TurnId: r.TurnID, Phase: r.Phase, CompletionJson: []byte(r.CompletionJSON)}
	}
	return v
}
