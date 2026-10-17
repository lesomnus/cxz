package websandbox

import "fmt"

// toolActivity emits a tool's whole life as the records an agent writes: the
// call, the bytes it produces while it runs, and the result that ends it. They
// share a request identity, which is what the reading projection groups them
// by -- so a client sees one row whose state moves from working to completed,
// rather than three rows it has to pair itself.
//
// The sandbox needed this because it had no paired tool at all: one bare
// `tool_result` with no call, so nothing here produced the row shape every
// client renders. That gap is why a change to where a tool card's output comes
// from went unnoticed by every browser test. See cxz#155.
//
// complete is false while a call is still running, which is the state a card
// shows as working and the only state live output belongs to.
func (s *Server) toolActivity(st *session, id string, complete bool) {
	request := st.value.GetStatus().GetRunId() + "/" + id
	command := "/usr/bin/zsh -lc 'ls -la ts/src'"
	s.event(st, "tool_call", "Bash", request, []byte(fmt.Sprintf(`{"simulated":true,"command":%q,"item":{"type":"commandExecution","command":%q,"status":"inProgress"}}`, command, command)))
	output := "total 48\ndrwxr-xr-x  app\ndrwxr-xr-x  features\ndrwxr-xr-x  shared\n"
	s.event(st, "tool_output", output, request, []byte(fmt.Sprintf(`{"simulated":true,"item":{"type":"commandExecution","status":"inProgress","aggregatedOutput":%q}}`, output)))
	if !complete {
		return
	}
	s.event(st, "tool_result", output, request, []byte(fmt.Sprintf(`{"simulated":true,"item":{"type":"commandExecution","command":%q,"status":"completed","exitCode":0,"aggregatedOutput":%q}}`, command, output)))
}

// skillActivity is a tool whose name is not what it is. "Skill" names every one
// of them; which skill and with what arguments is in the payload, which is why
// a row carries a description. Without one, this card read "Skill" and nothing
// else -- the gap that adding ToolSummary.kind/description closed.
func (s *Server) skillActivity(st *session, id string) {
	request := st.value.GetStatus().GetRunId() + "/" + id
	s.event(st, "tool_call", "Skill", request, []byte(`{"simulated":true,"input":{"skill":"dataviz","args":"palette"}}`))
	s.event(st, "tool_result", "Loaded the dataviz skill.", request, []byte(`{"simulated":true,"content":"Loaded the dataviz skill."}`))
}
