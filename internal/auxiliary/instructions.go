package auxiliary

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed instructions.md
var instructionsFile string

// What the prompt asks for. Summary and suggestion are counted in characters,
// because that is what a model can aim at, while the limits that throw a reply
// away are counted in bytes; TestPromptBudgetsFitTheLimits keeps the gap wide
// enough that Korean or emoji text cannot fall through it.
const (
	SummaryBullets   = 5
	BulletBudget     = 120
	SummaryBudget    = 1000
	SuggestionBudget = 500
	CheckpointBudget = 6000
)

// Instructions is the system prompt every auxiliary task shares; the task name in
// the message selects which fields of the reply are filled. It lives beside this
// file as Markdown so it can be read and edited as text rather than as one long Go
// string, and the budgets above are substituted into it rather than written there,
// so a limit that moves cannot leave the prompt asking for a reply that the
// decoder would then reject.
var Instructions = instructions()

func instructions() string {
	t, err := template.New("instructions").Parse(instructionsFile)
	if err == nil {
		var out strings.Builder
		if err = t.Execute(&out, budgets{SummaryBullets, BulletBudget, SummaryBudget, SuggestionBudget, CheckpointBudget}); err == nil {
			return strings.TrimSpace(out.String())
		}
	}
	// Only an edit to instructions.md reaches this: the file is embedded and the
	// data is constant, so a placeholder that names nothing fails at startup and
	// in every test of this package rather than silently reaching a provider.
	panic("auxiliary instructions: " + err.Error())
}

type budgets struct {
	SummaryBullets   int
	BulletBudget     int
	SummaryBudget    int
	SuggestionBudget int
	CheckpointBudget int
}
