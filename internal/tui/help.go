package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

type commandHelp struct {
	category, name, summary, detail, example string
}

var commandHelpEntries = []commandHelp{
	{"Session", "terminal", "Open the container terminal", "Opens a session-local container shell below the conversation (up to 24 rows). Ctrl+backtick opens it, focuses a visible panel, or folds a focused panel. Folding and session switching preserve the shell while this TUI stays attached. Click the composer to return without folding. Collapse is always clickable on the upper-right rule, even when a terminal application captures mouse input. Wheel or Alt+PgUp/PgDown browses shell history; click/drag the bottom track to seek. History is frozen while browsing so new output does not move it. The right end, Ctrl+End while browsing, or typing returns to live output. Alternate-screen applications keep their own wheel handling. Ctrl+C, Tab and other keys go to the shell while focused. Requires local Docker CLI access to the manager's engine. This is a remote-user workspace shell, not the agent's private authentication HOME. Exit the shell normally, then fold/reopen for a new shell. Leaving cxz closes the terminal connection; persistence across detach is not provided.", "/terminal\n/help terminal"},
	{"Inspection", "paste", "Preview or attach pasted text", "Pastes longer than 800 characters or three lines become indivisible chips. Full original text, including newlines, is sent by default. Left/Right selects a chip, then moves outside it. While selected, t chooses full-text mode, f uploads a private text file to session storage without opening a dialog, and d or Backspace/Delete removes the chip. Enter or Ctrl+P opens its preview. With no chip selected, t/f/d type normally. Pasted characters never execute these shortcuts. In the preview, Up/Down selects a paste, PgUp/PgDn scrolls, and t/f/d change mode or remove it. Escape closes the preview. Only chips still present in the current input are listed; deleted or sent chips are not restored from the cache. File mode sends its path and a read instruction, not the full text. Mode changes do not submit a message. Login codes and secret answers are excluded. Each paste/file is limited to 1 MiB; the 32 MiB memory cache is lost on detach. Uploaded files remain with session data after a chip is removed.", "/paste\n/help paste"},
	{"Inspection", "background", "Inspect background tasks", "Shows current-run provider task status, completion summaries and output paths in a live read-only overlay. Claude task snapshots are authoritative; launch results do not mark background work complete. Output files are not read automatically. Older telemetry loads separately from the transcript. Providers without task telemetry are not guessed from command text.", "/background"},
	{"Model settings", "model", "Inspect or select a provider model", "Lists provider-reported model IDs and supported effort levels. Use /model <id> to select an advertised model while idle; reset effort with /effort default before switching. Claude changes are confirmed by the CLI, Codex choices apply to the next turn. Confirmed preferences survive resume. Catalogs refresh every idle minute via Claude list_models or Codex model/list; old Claude CLIs may retain the initial catalog. A newer CLI requires an agent restart.", "/model\n/model <id-from-catalog>"},
	{"Model settings", "effort", "Inspect or select reasoning strength", "Uses one common command for Codex reasoning effort and Claude effort. Select a model first, then use a level that its catalog reports. /effort default clears the override. Claude's flag-settings transport cannot apply session-only max effort; unsupported controls are rejected without creating a chat turn.", "/effort\n/effort high\n/effort default"},
	{"Conversation", "answer", "Open an interactive question dialog", "Questions open automatically. /answer reopens the selected pending question. Up/Down or Tab moves; Space/Enter selects or toggles options. Other accepts a custom answer. Next/Back navigates questions; Submit sends all answers together. Ctrl+S advances/submits. Esc/Cancel closes without denying. PgUp/PgDn scrolls long descriptions and previews. /approval retains the raw payload. Codex asynchronous questions remain answerable after their turn ends; answers are sent as tool output, not approval RPC replies. Codex questions are single-select; Claude may request multiple selection. Advanced JSON replies are still accepted, but are not needed for the dialog.", "/answer\n/help answer"},
	{"Conversation", "context", "Inspect provider context", "Claude runs its native /context command and requires an idle session. Codex displays the last reported turn's token footprint and context window, not a live estimate. Missing provider data is not guessed.", "/context"},
	{"Conversation", "compact", "Compact agent context", "Requires an idle session. Runs native Claude /compact or Codex thread compaction. The provider reduces its conversation context; cxz keeps the full event journal. No arguments are accepted.", "/compact"},
	{"Permissions", "permission", "Choose manual or automatic approval", "ask requires manual approval. full automatically approves supported tool requests for this attached run, including pending ones; questions and unknown request types still need a reply. Full approval can allow file changes and shell commands. It resets on disconnect, run change or return to the project and does not change provider sandbox policy.", "/permission full\n/permission ask"},
	{"Permissions", "approval", "Inspect the selected request", "Shows the full selected pending approval payload locally. Tab focuses Pending approvals; arrow keys select a request. Enter allows it and Backspace denies it. Use /answer for a question that needs a structured response.", "/approval"},
	{"Inspection", "usage", "View session usage and account quota", "Reads session tokens, cost and duration from the full journal. Account quota is separate provider telemetry, queried at initialization, after turns and every minute while active. This command reads recorded data, not a fresh provider request. It also explains waiting, timeout, unsupported, unavailable and error states.", "/usage"},
	{"Inspection", "details", "Inspect the latest tool result", "Shows the full latest tool result locally instead of its compact transcript summary. Earlier raw events remain available through the cxz session events CLI command.", "/details"},
	{"Session", "stop", "Stop the current agent", "Stops the agent process, not just the current turn. Session history remains and Ctrl+R resumes a stopped session. To interrupt only the current turn, press Esc twice within three seconds; Ctrl+C instead detaches while the agent continues.", "/stop"},
	{"Session", "restart", "Restart this session's agent", "Submit /restart to open a confirmation dialog. Cancel is selected by default. Tab/Shift+Tab or arrows switch buttons; Enter selects; Esc cancels. Buttons also accept mouse clicks. Confirmation is bound to this session/run. Stops the active turn and clears pending approvals, then resumes the same session with its stored history and authentication. Automatic approval resets to manual. Failed stop never proceeds to resume; failed resume is reported without resending prompts. This does not update the manager/runtime or recreate the container. Older shared-profile sessions may require a new session and independent login.", "/restart"},
	{"Help", "help", "Browse help or a command reference", "Help is local and is never sent to the agent. Use /help for categories and shortcuts, or /help followed by a command name for its behavior and examples. Submit with Ctrl+S; Enter inserts a newline.", "/help\n/help answer\n/help permission"},
}

// Each key includes its own one-space padding. Alternating keycap backgrounds
// separate adjacent shortcut rows without painting the transcript background.
func helpKeycaps(keys string, row int) string {
	bg := lipgloss.CompleteColor{TrueColor: "#000000", ANSI256: "0", ANSI: "0"}
	if row%2 == 1 {
		bg = lipgloss.CompleteColor{TrueColor: "#101010", ANSI256: "0", ANSI: "0"}
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")).Background(bg)
	separator := style.Background(lipgloss.CompleteColor{TrueColor: map[bool]string{true: "#181818", false: "#080808"}[row%2 == 1], ANSI256: "0", ANSI: "0"})
	alternatives := strings.Split(keys, " / ")
	for i, chord := range alternatives {
		parts := strings.Split(chord, "+")
		for j, key := range parts {
			parts[j] = style.Render(" " + key + " ")
		}
		alternatives[i] = strings.Join(parts, separator.Render(" + "))
	}
	return strings.Join(alternatives, " / ")
}

func helpView(width int, topics ...string) string {
	w := max(1, width-2)
	topic := ""
	if len(topics) > 0 {
		topic = strings.TrimPrefix(strings.TrimSpace(topics[0]), "/")
	}
	if topic != "" {
		for _, entry := range commandHelpEntries {
			if entry.name == topic {
				return indentBlock(ansi.Hardwrap(lavender.Bold(true).Render("cxz /help "+topic)+"\n\n"+
					strong.Render(entry.summary)+"\n"+muted.Render(entry.detail)+"\n\n"+
					accent.Render("Examples")+"\n"+entry.example+"\n\n"+muted.Render("Submit with ")+helpKeycaps("Ctrl+S", 0), w, true))
			}
		}
		return indentBlock(ansi.Hardwrap("Unknown help topic: "+safeText(topic)+"\nUse /help to list commands; for example /help answer.", w, true))
	}
	profile := map[termenv.Profile]string{termenv.TrueColor: "24-bit True Color", termenv.ANSI256: "256 colors", termenv.ANSI: "16 colors", termenv.Ascii: "no color"}[lipgloss.ColorProfile()]
	lines := []string{lavender.Bold(true).Render("cxz /help"), muted.Render("Detected color profile: " + profile), muted.Render("Details and examples: /help <command> · e.g. /help answer")}
	category := ""
	for _, entry := range commandHelpEntries {
		if category != entry.category {
			category = entry.category
			lines = append(lines, "", accent.Bold(true).Render(category))
		}
		lines = append(lines, "/"+entry.name+"  "+muted.Render(entry.summary))
	}
	shortcuts := []struct{ category, keys, text string }{
		{"Input", "Ctrl+S", "Send message or command"},
		{"Input", "Ctrl+Enter", "Send message or command (Kitty keyboard protocol)"},
		{"Input", "Enter / Alt+Enter / Ctrl+J", "Newline"},
		{"Input", "Ctrl+X", "Clear draft"},
		{"Navigation", "Tab / Shift+Tab", "Next / previous focus: approvals → input → sessions"},
		{"Navigation", "↑ / ↓", "Select session (session focus) or slash hint"},
		{"Navigation", "PgUp / PgDn", "Scroll transcript (mouse wheel also works)"},
		{"Navigation", "Ctrl+End", "Follow latest output"},
		{"Navigation", "r", "Rename selected session; Enter saves, Esc cancels"},
		{"Navigation", "Ctrl+Q", "Focus project panel on wide terminals; otherwise return to project"},
		{"Navigation", "Ctrl+N", "Create session"},
		{"Approval controls", "↑ / ↓", "Select pending request (approval focus)"},
		{"Approval controls", "PgUp / PgDn", "Scroll full request (approval focus)"},
		{"Approval controls", "Enter / Backspace", "Allow / deny (approval focus)"},
		{"Approval controls", "F2 / F3", "Allow / deny selected request"},
		{"Agent controls", "Esc", "Press twice within 3s to interrupt current turn"},
		{"Agent controls", "F4", "Interrupt current turn immediately"},
		{"Agent controls", "Ctrl+R", "Resume stopped session"},
		{"Agent controls", "Ctrl+C", "Detach; agent continues"},
	}
	for i, shortcut := range shortcuts {
		if category != shortcut.category {
			category = shortcut.category
			lines = append(lines, "", accent.Bold(true).Render(category+" · keys"))
		}
		lines = append(lines, helpKeycaps(shortcut.keys, i)+"  "+muted.Render(shortcut.text))
	}
	return indentBlock(ansi.Hardwrap(strings.Join(lines, "\n"), w, true))
}
