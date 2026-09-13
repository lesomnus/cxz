package agentview

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"strings"
	"testing"
)

func TestQuestionProviderAdapters(t *testing.T) {
	for _, tc := range []struct {
		provider, method, raw, key string
		multi, other               bool
	}{
		{"claude", "AskUserQuestion", `{"input":{"questions":[{"header":"작업","question":"고르세요","multiSelect":true,"options":[{"label":"A","description":"설명","preview":"code"},{"label":"B"}]}]}}`, "고르세요", true, true},
		{"codex", "item/tool/requestUserInput", `{"params":{"questions":[{"id":"choice","question":"고르세요","options":[{"label":"A"},{"label":"B"}],"isOther":true}]}}`, "choice", false, true},
	} {
		qs, err := Questions(tc.provider, tc.method, []byte(tc.raw))
		if err != nil || len(qs) != 1 {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		q := qs[0]
		if q.Key != tc.key || q.Multi != tc.multi || q.Other != tc.other {
			t.Fatalf("%+v", q)
		}
		encoded, err := EncodeQuestionAnswers(qs, [][]bool{{true, tc.multi}}, []string{""})
		if err != nil {
			t.Fatal(err)
		}
		var answers map[string]core.AnswerSelection
		json.Unmarshal([]byte(encoded), &answers)
		want := "A"
		if tc.multi {
			want = "A, B"
		}
		if strings.Join(answers[tc.key].Selected, ", ") != want {
			t.Fatal(encoded)
		}
		if _, err := EncodeQuestionAnswers(qs, [][]bool{{false, false}}, []string{""}); err == nil {
			t.Fatal("empty answer accepted")
		}
	}
}

func TestQuestionMalformedAndSecret(t *testing.T) {
	for _, raw := range []string{`{}`, `{"input":{"questions":[]}}`, `{"input":{"questions":[{"question":"x"},{"question":"x"}]}}`} {
		if _, err := Questions("claude", "AskUserQuestion", []byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	qs, err := Questions("codex", "item/tool/requestUserInput", []byte(`{"params":{"questions":[{"id":"secret","question":"Token?","isSecret":true}]}}`))
	if err != nil || !qs[0].Secret || !qs[0].Other {
		t.Fatalf("%+v %v", qs, err)
	}
}

func TestOtherMatchesOptionAndTrims(t *testing.T) {
	qs := []Question{{Key: "q", Text: "q", Multi: true, Other: true, Options: []QuestionOption{{Label: "high"}}}}
	for _, selected := range [][]string{nil, {"high"}} {
		got, err := NormalizeAnswers(qs, map[string]core.AnswerSelection{"q": {Selected: selected, Other: "  high  "}})
		if err != nil || len(got["q"].Selected) != 1 || got["q"].Other != "" {
			t.Fatalf("%+v %v", got, err)
		}
	}
}
