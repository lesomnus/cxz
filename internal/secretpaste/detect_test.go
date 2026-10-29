package secretpaste

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWholeTokens(t *testing.T) {
	body, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Text, Kind string }
	if err := json.Unmarshal(body, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			if got := Detect(tt.Text); got != tt.Kind {
				t.Fatalf("kind = %q, want %q", got, tt.Kind)
			}
		})
	}
}
