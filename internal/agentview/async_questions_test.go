package agentview

import "testing"

func TestAsyncQuestionNormalization(t *testing.T) {
	raw := []byte(`{"item":{"id":"call","type":"agentMessage","delivery":"async","questions":[{"title":"Choose","options":["A","B"]},{"title":"Choose","options":null}]}}`)
	qs, err := Questions("codex", CodexAsyncQuestion, raw)
	if err != nil || len(qs) != 2 || qs[0].Key == qs[1].Key || qs[0].Multi || !qs[0].Other {
		t.Fatal(qs, err)
	}
	for _, raw := range []string{
		`{"item":{"id":"call","type":"agentMessage","delivery":null,"questions":[{"title":"X"}]}}`,
		`{"item":{"id":"call","type":"agentMessage","delivery":"async","questions":[{"title":"X","options":["a","a"]}]}}`,
		`{"item":{"id":"call","type":"agentMessage","delivery":"async","questions":[]}}`,
	} {
		if _, err := CodexAsyncQuestions([]byte(raw)); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
}
