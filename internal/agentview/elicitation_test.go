package agentview

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"testing"
)

const form = `{"params":{"serverName":"cxz_memory","mode":"form","message":"Store memory?","requestedSchema":{"type":"object","properties":{"name":{"type":"string","minLength":2},"count":{"type":"integer","minimum":1},"enabled":{"type":"boolean"},"kind":{"type":"string","oneOf":[{"const":"a","title":"Alpha"},{"const":"b","title":"Beta"}]},"optional":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["name","count","enabled","kind","tags"]}}}`

func answers() map[string]core.AnswerSelection {
	return map[string]core.AnswerSelection{
		"cxz:action": {Selected: []string{"Accept"}}, "field:name": {Other: "한글"}, "field:count": {Other: "2"}, "field:enabled": {Selected: []string{"false"}}, "field:kind": {Selected: []string{"b"}}, "field:optional": {}, "field:tags": {Other: `["one","two"]`},
	}
}
func TestElicitationTypedForm(t *testing.T) {
	qs, err := ElicitationQuestions([]byte(form))
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 7 || !qs[0].ElicitationAction || qs[1].Key != "field:count" {
		t.Fatal(qs)
	}
	r, err := ElicitationResponse([]byte(form), true, answers(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	var out struct{ Content map[string]any }
	json.Unmarshal(b, &out)
	if r["action"] != "accept" || out.Content["enabled"] != false || out.Content["count"] != float64(2) || out.Content["kind"] != "b" {
		t.Fatal(string(b))
	}
	if _, ok := out.Content["optional"]; ok {
		t.Fatal("optional field was invented")
	}
	for _, bad := range []string{"0", "1.5", "true", "not-json"} {
		v := answers()
		v["field:count"] = core.AnswerSelection{Other: bad}
		if _, err := ElicitationResponse([]byte(form), true, v, nil); err == nil {
			t.Fatal("invalid number accepted", bad)
		}
	}
}
func TestElicitationDeclineCancelAndURL(t *testing.T) {
	for _, action := range []string{"Decline", "Cancel"} {
		r, err := ElicitationResponse([]byte(form), true, map[string]core.AnswerSelection{"cxz:action": {Selected: []string{action}}}, nil)
		if err != nil || r["content"] != nil || r["action"] == "accept" {
			t.Fatal(r, err)
		}
	}
	raw := []byte(`{"params":{"serverName":"external","mode":"url","message":"Login","url":"https://example.com/auth","elicitationId":"id"}}`)
	r, err := ElicitationResponse(raw, true, map[string]core.AnswerSelection{"cxz:action": {Selected: []string{"Accept"}}}, nil)
	if err != nil || r["action"] != "accept" || r["content"] != nil {
		t.Fatal(r, err)
	}
	if _, err := ElicitationResponse(raw, true, nil, nil); err == nil {
		t.Fatal("missing explicit decision accepted")
	}
	r, err = ElicitationResponse([]byte(`invalid`), false, nil, nil)
	if err != nil || r["action"] != "decline" {
		t.Fatal("cannot decline malformed request")
	}
}
func TestElicitationEmptyFormAndRemoteSchema(t *testing.T) {
	for _, schema := range []string{`{"type":"object","properties":{}}`, `{"type":"object","$ref":"https://example.com/schema"}`} {
		raw := []byte(`{"params":{"mode":"form","requestedSchema":` + schema + `}}`)
		r, err := ElicitationResponse(raw, true, map[string]core.AnswerSelection{"cxz:action": {Selected: []string{"Accept"}}}, nil)
		if schema == `{"type":"object","properties":{}}` {
			if err != nil || r["content"] == nil {
				t.Fatal(r, err)
			}
		} else if err == nil {
			t.Fatal("remote reference fetched or ignored")
		}
	}
}

func TestMCPDecisionClassificationUsesPayload(t *testing.T) {
	for _, tt := range []struct {
		raw           string
		buttons, auto bool
	}{
		{`{"params":{"mode":"form","requestedSchema":{"type":"object","properties":{}}}}`, true, true},
		{`{"params":{"mode":"url","url":"https://example.com"}}`, true, false},
		{form, false, false},
		{`{"params":{"mode":"form","requestedSchema":{"type":"object","properties":{"optional":{"type":"string"}}}}}`, false, false},
		{`{"params":{"mode":"form","requestedSchema":{"type":"object","properties":{},"required":["missing"]}}}`, false, false},
		{`{"params":{"mode":"form","requestedSchema":{"type":"object","minProperties":1}}}`, false, false},
		{`{"params":{"mode":"unknown"}}`, false, false},
	} {
		raw := []byte(tt.raw)
		if ElicitationButtons(raw) != tt.buttons || AutomaticApproval(CodexElicitation, raw) != tt.auto || QuestionRequest(CodexElicitation, raw) == tt.buttons {
			t.Fatal(tt)
		}
	}
}
