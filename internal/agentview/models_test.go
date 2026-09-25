package agentview

import (
	"slices"
	"testing"
)

func TestClaudeDefaultAndResolvedModelCapabilities(t *testing.T) {
	// Claude 2.1.267/2.1.278 report a default entry without isDefault. Its
	// resolvedModel is also the ID returned by get_settings.applied.model.
	models := Models("claude", []byte(`{"models":[
		{"value":"default","resolvedModel":"claude-opus-5[1m]","supportedEffortLevels":["low","medium","high","xhigh","max"]},
		{"value":"sonnet","resolvedModel":"claude-sonnet-5","supportedEffortLevels":["low","medium","high","xhigh","max"]},
		{"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001"}
	]}`))
	for _, tc := range []struct{ selected, applied, want string }{
		{"", "", "default"},
		{"default", "", "default"},
		{"", "claude-opus-5[1m]", "default"},
		{"claude-sonnet-5", "", "sonnet"},
		{"sonnet", "claude-sonnet-5", "sonnet"},
		{"sonnet", "claude-opus-5[1m]", "default"},
		{"", "not-in-catalog", ""},
	} {
		got, ok := SelectedModel(models, tc.selected, tc.applied)
		if ok != (tc.want != "") || got.ID != tc.want {
			t.Fatalf("%+v: selected %+v (found=%t)", tc, got, ok)
		}
		if ok && !slices.Contains(got.Efforts, "high") {
			t.Fatal("lost reasoning choices", got)
		}
	}
	if !models[0].Default {
		t.Fatal("Claude default was not normalized")
	}
	if option, ok := SelectedModel(models, "haiku", ""); !ok || len(option.Efforts) != 0 {
		t.Fatal("invented reasoning capabilities", option)
	}
	legacy := Models("claude", []byte(`{"models":[{"value":"sonnet","supportedEffortLevels":["low","high"]}]}`))
	if option, ok := SelectedModel(legacy, "sonnet", "claude-sonnet-5"); !ok || option.ID != "sonnet" {
		t.Fatal("legacy explicit alias lost its capabilities")
	}
}
