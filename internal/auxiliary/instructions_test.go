package auxiliary

import (
	"strconv"
	"strings"
	"testing"
)

// The prompt is a file now, so an empty or renamed field is a silent behavior
// change rather than a compile error: the provider would be handed a system
// prompt that no longer matches the output schema it is asked to satisfy.
func TestInstructionsCoverEveryTask(t *testing.T) {
	if strings.TrimSpace(Instructions) != Instructions || Instructions == "" {
		t.Fatal("embedded instructions are empty or not trimmed")
	}
	for _, field := range []string{"summary", "suggestion", "checkpoint", "combined"} {
		if !strings.Contains(Instructions, field) {
			t.Fatal("instructions never mention", field)
		}
	}
	// An unsubstituted placeholder would reach the model verbatim. Template
	// execution fails on an unknown field, so this catches the other direction:
	// a budget written into the file by hand instead of substituted.
	if strings.Contains(Instructions, "{{") {
		t.Fatal("instructions carry an unrendered template action")
	}
	for _, budget := range []int{SummaryBullets, BulletBudget, SummaryBudget, SuggestionBudget, CheckpointBudget} {
		if !strings.Contains(Instructions, strconv.Itoa(budget)) {
			t.Fatal("instructions never state the budget", budget)
		}
	}
}

// The prompt asks for characters and the decoder counts bytes, so a budget has to
// survive its own worst case: a reply that obeys the prompt must never be thrown
// away for being too large, however the text is encoded.
func TestPromptBudgetsFitTheLimits(t *testing.T) {
	const maxBytesPerChar = 4
	for _, c := range []struct {
		field  string
		budget int
		limit  int
	}{
		// A retained summary is clipped, not rejected, but a clip would cut the
		// last bullet off the history the suggestion call reads back.
		{"summary", SummaryBudget * maxBytesPerChar, min(SummaryLimit, RetainedSummaryLimit)},
		{"suggestion", SuggestionBudget * maxBytesPerChar, SuggestionLimit},
		{"checkpoint", CheckpointBudget, CheckpointLimit},
		// A combined call returns the summary and the suggestion in one reply.
		{"combined reply", (SummaryBudget + SuggestionBudget) * maxBytesPerChar, MaxOutput},
	} {
		if c.budget > c.limit {
			t.Fatal(c.field, "may reach", c.budget, "bytes, over the limit of", c.limit)
		}
	}
	if SummaryBullets*BulletBudget > SummaryBudget {
		t.Fatal("the per-bullet budget does not fit the summary budget")
	}
}
