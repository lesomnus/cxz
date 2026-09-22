package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func TestCompactProviderQuota(t *testing.T) {
	now := time.Unix(1900000000, 0)
	all := agentview.Quota("codex", "account/rateLimits/updated", []byte(`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":64,"windowDurationMins":10080,"resetsAt":1900500000}},"spark":{"limitName":"GPT-5.3-Codex-Spark","primary":{"usedPercent":0,"windowDurationMins":300,"resetsAt":1900018000},"secondary":{"usedPercent":2,"windowDurationMins":10080,"resetsAt":1900500000}}}}`))
	for i := range all {
		all[i].Observed = now
	}
	m := conversationModel()
	m.current().Agent = "codex"
	m.current().Model = "gpt-normal"
	m.quotaWindows = all
	text := ansi.Strip(m.quotaStatus(now, 100))
	if !strings.Contains(text, "36%") || !strings.Contains(text, "+2 limits") || strings.Contains(text, "Spark") || strings.Contains(text, "100%") {
		t.Fatal(text)
	}
	m.current().Model = "gpt-5.3-codex-spark"
	text = ansi.Strip(m.quotaStatus(now, 100))
	if strings.Contains(text, "36%") || !strings.Contains(text, "100%") || !strings.Contains(text, "98%") || !strings.Contains(text, "+1 limits") || strings.Contains(text, "Spark/") {
		t.Fatal(text)
	}
	for width := 8; width <= 100; width++ {
		if got := m.quotaStatus(now, width); ansi.StringWidth(got) > width {
			t.Fatalf("width %d: %s", width, got)
		}
	}
	compact := ansi.Strip(m.quotaStatus(now, 40))
	if strings.Contains(compact, "⣿") || !strings.Contains(compact, "100% 5h 5h") || !strings.Contains(compact, "98% wk") {
		t.Fatal(compact)
	}
	if all[1].Label == "5h" {
		t.Fatal("full report labels mutated")
	}
}

func TestQuotaDefaultModelAndRunIsolation(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	m.events["s"] = []*api.Event{
		{Kind: "models", RunId: "run", Payload: []byte(`{"Model":"","Models":[{"ID":"gpt-5.3-codex-spark","Default":true}]}`)},
		{Kind: "models", RunId: "old", Payload: []byte(`{"Model":"wrong"}`)},
	}
	if m.quotaModel() != "gpt-5.3-codex-spark" {
		t.Fatal(m.quotaModel())
	}
	w, folded := statusQuotaWindows("claude", "", []agentview.Window{{Key: "seven_day_opus", Label: "wk-opus"}, {Key: "seven_day", Label: "wk"}, {Key: "five_hour", Label: "5h"}})
	if len(w) != 2 || w[0].Label != "5h" || folded != 1 {
		t.Fatal(w, folded)
	}
}

func TestQuotaIdentitySpaceAndCountdown(t *testing.T) {
	m := conversationModel()
	m.current().Alias = "pulse"
	m.current().Agent = "codex"
	m.current().Model = "default"
	m.current().Account = "main"
	n := 36.0
	now := time.Now()
	m.quotaWindows = []agentview.Window{{Key: "codex/primary", Bucket: "codex", Label: "wk", Remaining: &n, Observed: now, Reset: now.Add(24 * time.Hour)}}
	m.width = 80
	rows := strings.Split(ansi.Strip(m.sessionScreen()), "\n")
	last := rows[len(rows)-1]
	if strings.Contains(last, "pulse") || strings.Contains(last, "codex/default") || strings.Contains(last, "main") || !strings.Contains(last, "36%") || !strings.HasSuffix(last, " ") {
		t.Fatal(last)
	}
	if quotaCountdown(now.Add(5*time.Hour), now) != "5h" || quotaCountdown(now.Add(48*time.Hour), now) != "2d" {
		t.Fatal("zero units retained")
	}
}
