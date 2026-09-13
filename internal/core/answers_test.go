package core

import "testing"

func TestDecodeStructuredAnswers(t *testing.T) {
	old, choices, err := DecodeAnswers(`{"q":{"selected":["low","a, b"],"other":"hello, world"}}`)
	if err != nil || len(old) != 0 || len(choices["q"].Selected) != 2 || choices["q"].Other != "hello, world" {
		t.Fatalf("%+v %v", choices, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"q":null}`, `{"q":{"selectd":["x"]}}`, `{"q":"x","r":{"selected":["y"]}}`} {
		if _, _, err := DecodeAnswers(raw); err == nil {
			t.Fatal(raw)
		}
	}
}
