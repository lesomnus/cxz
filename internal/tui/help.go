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
	{"Inline commands", "@redact", "Insert a secret inside your message", "Type @ at the start of input or after whitespace. Enter on @redact opens hidden input; Tab completes its name. Enter in the dialog replaces only that command with a [Redacted] chip. Ctrl+X clears; Esc restores the draft and cursor. Normal Enter remains newline; Ctrl+S sends. At send time, a 0600 file is created in host tmpfs and only its path is sent. Limit: 64 KiB per secret. Files survive TUI, agent, manager and project container shutdown/recreation. The manager sweeps at startup and hourly, deleting files idle for 8 hours using the later of access/modify timestamps; access time is approximate. Host OS reboot clears tmpfs. Existing installations need manager and project bind-mount upgrades. No normal paste preview or attachment is available. The agent, same-UID processes and root can read files; printing a secret can expose it to provider context/logs. tmpfs may swap. Unsent secret input remains memory-only and is lost on TUI exit.", "Use this token: @redact\n/help redact"},
	{"Session", "terminal", "Open the container terminal", "Opens a session-local container shell below the conversation (up to 24 rows). Ctrl+backtick opens it, focuses a visible panel, or folds a focused panel. Folding and session switching preserve the shell while this TUI stays attached. Click the composer to return without folding. Collapse is always clickable on the upper-right rule, even when a terminal application captures mouse input. Wheel or Alt+PgUp/PgDown browses shell history; click/drag the bottom track to seek. History is frozen while browsing so new output does not move it. The right end, Ctrl+End while browsing, or typing returns to live output. Alternate-screen applications keep their own wheel handling. Ctrl+C, Tab and other keys go to the shell while focused. Requires local Docker CLI access to the manager's engine. This is a remote-user workspace shell, not the agent's private authentication HOME. Exit code 0 automatically folds the panel and returns to chat. Nonzero exits keep the panel visible. Reopen an exited panel to start a new shell. Leaving cxz closes the terminal connection; persistence across detach is not provided.", "/terminal\n/help terminal"},
	{"Inspection", "paste", "Preview or attach pasted text", "Pastes longer than 800 characters or three lines become indivisible chips. Full original text, including newlines, is sent by default. Left/Right selects a chip, then moves outside it. While selected, t chooses full-text mode, f uploads a private text file to session storage without opening a dialog, and d or Backspace/Delete removes the chip. Enter or Ctrl+P opens its preview. With no chip selected, t/f/d type normally. Pasted characters never execute these shortcuts. In the preview, Up/Down selects a paste, PgUp/PgDn scrolls, and t/f/d change mode or remove it. Escape closes the preview. Only chips still present in the current input are listed; deleted or sent chips are not restored from the cache. File mode sends its path and a read instruction, not the full text. Mode changes do not submit a message. Login codes and secret answers are excluded. Each paste/file is limited to 1 MiB; the 32 MiB memory cache is lost on detach. Uploaded files remain with session data after a chip is removed.", "/paste\n/help paste"},
	{"Inspection", "logs", "Inspect session or project diagnostics", "Opens a scrollable, wrapped diagnostic report. /logs shows current-session TUI notices, journal diagnostics, supervisor and agent stderr. /logs project adds other sessions, runtime/provisioning and this TUI’s Wisp diagnostics. Recent tails are bounded and unavailable sources are labeled. TUI/Wisp history is local to this TUI lifetime. Press r to refresh, Home/End to jump, Esc to close. This is a snapshot, not a live stream.", "/logs\n/logs project"},
	{"Inspection", "background", "Inspect background tasks", "Shows current-run provider task status, completion summaries and output paths in a live read-only overlay. Claude task snapshots are authoritative; launch results do not mark background work complete. Output files are not read automatically. Older telemetry loads separately from the transcript. Providers without task telemetry are not guessed from command text.", "/background"},
	{"Model settings", "model", "Inspect or select a provider model", "Lists provider-reported model IDs and supported effort levels. Use /model <id> to select an advertised model while idle; reset effort with /effort default before switching. Claude changes are confirmed by the CLI, Codex choices apply to the next turn. Confirmed preferences survive resume. Catalogs refresh every idle minute via Claude list_models or Codex model/list; old Claude CLIs may retain the initial catalog. A newer CLI requires an agent restart.", "/model\n/model <id-from-catalog>"},
	{"Model settings", "effort", "Inspect or select reasoning strength", "Uses one common command for Codex reasoning effort and Claude effort. Select a model first, then use a level that its catalog reports. /effort default clears the override. Claude's flag-settings transport cannot apply session-only max effort; unsupported controls are rejected without creating a chat turn.", "/effort\n/effort high\n/effort default"},
	{"Conversation", "answer", "Open an interactive question dialog", "Questions open automatically. /answer reopens the selected pending question. Up/Down or Tab moves; Space/Enter selects or toggles options. Other accepts a custom answer. Next/Back navigates questions; Submit sends all answers together. Ctrl+S advances/submits. Esc/Cancel closes without denying. PgUp/PgDn scrolls long descriptions and previews. /approval retains the raw payload. Codex asynchronous questions remain answerable after their turn ends; answers are sent as tool output, not approval RPC replies. Codex questions are single-select; Claude may request multiple selection. Advanced JSON replies are still accepted, but are not needed for the dialog.", "/answer\n/help answer"},
	{"Conversation", "context", "Inspect provider context", "Claude runs its native /context command and requires an idle session. Codex displays the last reported turn's token footprint and context window, not a live estimate. Missing provider data is not guessed.", "/context"},
	{"Conversation", "compact", "Compact agent context", "Requires an idle session. Runs native Claude /compact or Codex thread compaction. The provider reduces its conversation context; cxz keeps the full event journal. No arguments are accepted.", "/compact"},
	{"Permissions", "permission", "Choose manual or automatic approval", "ask saves manual approval. full saves automatic approval for this session: its supervisor handles supported pending and future tool requests even when another session is viewed or the TUI is closed. The policy survives restart/resume and is shown from server state. New sessions and sessions without a saved policy start in full mode; explicitly saved ask is preserved. Questions and unknown request types still need a reply. Use /permission ask to disable it; decisions already sent cannot be retracted. Provider sandbox settings are unchanged", "/permission full\n/permission ask"},
	{"Permissions", "approval", "Inspect the selected request", "Shows the full selected pending approval payload locally. Tab focuses Pending approvals; arrow keys select a request. Enter allows it and Backspace denies it. Use /answer for a question that needs a structured response.", "/approval"},
	{"Inspection", "usage", "View session usage and account quota", "Reads session tokens, cost and duration from the full journal. Account quota is separate provider telemetry, queried at initialization, after turns and every minute while active. This command reads recorded data, not a fresh provider request. It also explains waiting, timeout, unsupported, unavailable and error states.", "/usage"},
	{"Inspection", "view", "Select and inspect tool activity", "Select a rendered tool row with Up/Down, then Enter to open the same preview as a mouse click. Read and other results use recorded output; Write/Edit use submitted content/diffs. Home/End selects the first/last loaded tool. Up at the first tool loads older history when available. Enter focuses the dark-gray preview with one-cell padding and a bottom focus rule; arrows/PgUp/PgDn/Home/End scroll it, x or Esc closes it, and Tab returns to selection. Esc in selection returns to the composer. Mouse-opened previews also receive keyboard focus; Tab returns to input. Pasted keys never trigger preview actions.", "/view"},
	{"Inspection", "details", "Inspect the latest tool result", "Shows the full latest tool result locally instead of its compact transcript summary. Use /view or click a tool row to open its shaded preview. Write/Edit show highlighted input; Read and other results show recorded output. Scroll with arrows or the mouse wheel; x, Esc or × closes the focused panel. Earlier raw events remain available through the cxz session events CLI command.", "/details"},
	{"Session", "stop", "Stop the current agent", "Stops the agent process, not just the current turn. Session history remains and Ctrl+R resumes a stopped session. To interrupt only the current turn, press Esc twice within three seconds; Ctrl+C instead detaches while the agent continues.", "/stop"},
	{"Session", "restart", "Restart this session's agent", "Submit /restart to open a confirmation dialog. Cancel is selected by default. Tab/Shift+Tab or arrows switch buttons; Enter selects; Esc cancels. Buttons also accept mouse clicks. Confirmation is bound to this session/run. Stops the active turn and clears pending approvals, then resumes the same session with its stored history and authentication. The saved permission policy is preserved. Failed stop never proceeds to resume; failed resume is reported without resending prompts. This does not update the manager/runtime or recreate the container. Older shared-profile sessions may require a new session and independent login.", "/restart"},
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
			if strings.TrimPrefix(entry.name, "@") == strings.TrimPrefix(topic, "@") {
				return indentBlock(ansi.Hardwrap(lavender.Bold(true).Render("cxz /help "+topic)+"\n\n"+
					strong.Render(entry.summary)+"\n"+muted.Render(entry.detail)+"\n\n"+
					accent.Render("Examples")+"\n"+entry.example+"\n\n"+muted.Render("Submit with ")+helpKeycaps("Ctrl+S", 0), w, true))
			}
		}
		return indentBlock(ansi.Hardwrap("Unknown help topic: "+safeText(topic)+"\nUse /help to list commands; for example /help answer.", w, true))
	}
	profile := map[termenv.Profile]string{termenv.TrueColor: "24-bit True Color", termenv.ANSI256: "256 colors", termenv.ANSI: "16 colors", termenv.Ascii: "no color"}[lipgloss.ColorProfile()]
	lines := []string{lavender.Bold(true).Render("cxz /help"), muted.Render("Detected color profile: " + profile), muted.Render("Details and examples: /help <command> · e.g. /help answer")}
	lines = append(lines, muted.Render("Container paths: backtick + / or ~ · arrows select · Tab browses · Enter completes and closes · Esc dismisses"))
	category := ""
	for _, entry := range commandHelpEntries {
		if category != entry.category {
			category = entry.category
			lines = append(lines, "", accent.Bold(true).Render(category))
		}
		name := entry.name
		if !strings.HasPrefix(name, "@") {
			name = "/" + name
		}
		lines = append(lines, name+"  "+muted.Render(entry.summary))
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
		{"Navigation", "Ctrl+Q", "Project list: n new, a accounts, s stop, d delete; sidebar or full view"},
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
