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
	{"Conversation", "title", "Generate or set a session title", "Configure Session title in Settings → AI tasks. New sessions receive a draft after their first input and a final title after three successful turns. /title regenerates using bounded retained conversation and ends automatic naming. /title set <title> assigns a manual title that automatic generation never overwrites. Alias and UUID stay unchanged.", "/title\n/title set CI performance"},
	{"Conversation", "summary", "Inline AI summary", "Set an account/model in Settings → AI tasks. /summary on or off overrides the default for this session. /summary alone generates once while off, and does nothing while on. The summary and animated loading dots appear below the final response without a dialog. A one-shot reads bounded retained server history; automatic mode processes new completed turns only. Auxiliary usage remains available through cxz ai status.", "/summary on\n/summary off\n/summary"},
	{"Conversation", "suggest", "AI next-message ghost", "/suggest on or off changes only this session. /suggest alone generates once while off, and does nothing while on. The suggestion or animated loading dots use the empty composer's placeholder color and disappear while typing. Right arrow accepts the visible suggestion into an empty composer for editing. Alt+G checks freshness with the server before copying. Neither sends the message. Matching task profiles generate summary and suggestion together; separate suggestion calls include an available same-turn summary.", "/suggest on\n/suggest off\n/suggest"},
	{"Inline commands", "attach", "Attach a host file inside your message", "Type a backtick immediately followed by !, then drop, paste or type an absolute host file path. Host means the computer running the cxz client. After 300 ms without changes, readable files become chips at that position. A closing backtick triggers an immediate check. Plain paths are never automatically uploaded; backticks without ! keep browsing the session container. Host suggestions read the client filesystem: Tab browses, Enter opens a directory or finishes a file. Multiple quoted paths are supported in one marker. Incomplete or missing paths stay editable; Ctrl+S waits until references have become chips and uploads have finished. Chips use the Usage progress bar, then a size of at most five characters. Left/Right selects a chip, Enter shows details, f retries, and d removes it. Each regular file is limited to 1 GiB. Existing projects need the read-only asset mount. Uploaded files remain available after chip removal.", "Compare `!/home/me/report.pdf` with the current result.\nWindows: `!\"C:\\Users\\me\\My Report.pdf\"`\n/help attach"},
	{"Inspection", "record", "Record TUI diagnostics", "F9 or /record toggles debug recording; a button is also available in Ctrl+. settings. Stop saves a private JSONL file under the client state directory’s recordings folder; settings or /record status shows its full path. Recording covers this TUI process across session switches, with a red REC label above the input and a dot that blinks every second. It includes sanitized navigation sequences, decoded key categories, focus and cursor changes, async event types and render timing. Typed/pasted text, conversation content, credentials and raw screen output are excluded. Up to 12000 recent events are retained, with a dropped-event count. Leaving the TUI also saves an active recording. Failed saves retain the recording for retry while the TUI stays open.", "/record"},
	{"Inspection", "memory", "Shared project memory", "Browse provider-neutral Markdown memories and saved copies. Enter opens; / searches; s saves your current written memory; c copies the selected memory; n starts a session from a frozen copy after account selection. Space selects collections and m combines them without rewriting their contents. e renames; d deletes after confirmation; t opens trash; u restores; x permanently deletes trash after confirmation. i imports native Markdown memory and instructions, excluding credentials and transcripts. o opens the original agent file browser. r refreshes; external edits are also picked up by periodic refresh. Memory is stored on the project runtime host, including for remote connections. Compact is not included. Existing agents need restarting to receive shared-memory MCP tools.", "/memory"},
	{"Settings", "settings", "Shared Docker status and maintenance", "Ctrl+. opens settings from the conversation or project list. Shows client and connected Manager versions and channels, upstream edge/stable releases, server history retention and client scroll-window limits, and shared engine status and build cache usage. Engine status refreshes every ten seconds; upstream metadata refreshes every minute or on manual refresh. Activate starts the engine with the manager’s saved settings; the same button becomes Deactivate while running. Disabled buttons are skipped by keyboard navigation. Deactivate and cache cleanup require confirmation; cleanup removes unused build cache only. Images and volumes are retained. Esc or Ctrl+. returns. Edit configuration with cxz edit.", "/settings"},
	{"Inline commands", "@alias", "Mention another session", "Type @ at a word boundary to show other sessions in this project in a grid. Type to filter; arrows select, PgUp/PgDown page, Enter or Tab inserts, and Esc closes. Mouse hover, click and wheel work too. The selected session title and agent appear below the grid. Mentions remain plain text; the agent resolves the alias through session_lookup. Email addresses, code and pasted mentions do not trigger completion.", "Refer to @seal for the earlier decision"},
	{"Inline commands", "/@redact", "Insert a secret inside your message", "Type /@ at the start of input or after whitespace. Enter on /@redact opens hidden input; Tab completes its name. Enter in the dialog replaces only that command with a [Redacted] chip. Ctrl+X clears; Esc restores the draft and cursor. Normal Enter remains newline; Ctrl+S sends. At send time, a 0600 file is created in host tmpfs and only its path is sent. Limit: 64 KiB per secret. Files survive TUI, agent, manager and project container shutdown/recreation. The manager sweeps at startup and hourly, deleting files idle for 8 hours using the later of access/modify timestamps; access time is approximate. Host OS reboot clears tmpfs. Over an ssh connection the manager writes the file, so the secret crosses only that encrypted link; over the exposed plaintext TCP surface it is refused. Existing installations need manager and project bind-mount upgrades. No normal paste preview or attachment is available. The agent, same-UID processes and root can read files; printing a secret can expose it to provider context/logs. tmpfs may swap. Unsent secret input remains memory-only and is lost on TUI exit.", "Use this token: /@redact\n/help redact"},
	{"Session", "terminal", "Open the container terminal", "Opens a session-local container shell below the conversation (up to 24 rows). Ctrl+backtick opens it, focuses a visible panel, or folds a focused panel. Folding and session switching preserve the shell while this TUI stays attached. Click the composer to return without folding. Collapse is always clickable on the upper-right rule, even when a terminal application captures mouse input. Wheel or Alt+PgUp/PgDown browses shell history; click/drag the bottom track to seek. History is frozen while browsing so new output does not move it. The right end, Ctrl+End while browsing, or typing returns to live output. Alternate-screen applications keep their own wheel handling. Ctrl+C, Tab and other keys go to the shell while focused. Requires local Docker CLI access to the manager's engine. This is a remote-user workspace shell, not the agent's private authentication HOME. Exit code 0 automatically folds the panel and returns to chat. Nonzero exits keep the panel visible. Reopen an exited panel to start a new shell. Leaving cxz closes the terminal connection; persistence across detach is not provided.", "/terminal\n/help terminal"},
	{"Inspection", "paste", "Preview or attach pasted text", "Pastes longer than 800 characters or three lines become indivisible chips. Full original text, including newlines, is sent by default. Left/Right selects a chip, then moves outside it. While selected, t chooses full-text mode, f uploads a private text file to session storage without opening a dialog, and d or Backspace/Delete removes the chip. Enter or Ctrl+P opens its preview. With no chip selected, t/f/d type normally. Pasted characters never execute these shortcuts. In the preview, Up/Down selects a paste, PgUp/PgDn scrolls, and t/f/d change mode or remove it. Press r in the preview to replace the selected chip with editable original text and close the preview; the cursor moves to the end of that text. If the input cannot preserve the full text, the chip is kept. Escape closes the preview. Only chips still present in the current input are listed; deleted or sent chips are not restored from the cache. File mode sends its path and a read instruction, not the full text. Mode changes do not submit a message. Login codes and secret answers are excluded. Each paste/file is limited to 1 MiB; the 32 MiB memory cache is lost on detach. Uploaded files remain with session data after a chip is removed.", "/paste\n/help paste"},
	{"Inspection", "logs", "Inspect session or project diagnostics", "Opens a scrollable, wrapped diagnostic report. /logs shows current-session TUI notices, journal diagnostics, supervisor and agent stderr. /logs project adds other sessions, runtime/provisioning and this TUI’s Wisp diagnostics. Recent tails are bounded and unavailable sources are labeled. TUI/Wisp history is local to this TUI lifetime. Press r to refresh, Home/End to jump, Esc to close. This is a snapshot, not a live stream.", "/logs\n/logs project"},
	{"Inspection", "background", "Inspect background tasks", "Shows current-run provider task status, completion summaries and output paths in a live read-only overlay. Claude task snapshots are authoritative; launch results do not mark background work complete. Output files are not read automatically. Older telemetry loads separately from the transcript. Providers without task telemetry are not guessed from command text.", "/background"},
	{"Model settings", "model", "Inspect or select a provider model", "Lists provider-reported model IDs and supported reasoning levels. Use /model <id> to select an advertised model while idle; reset effort with /effort default before switching. Claude changes are confirmed by the CLI, Codex choices apply to the next turn. cxz stores confirmed preferences in the session journal and restores them on resume. Catalogs refresh every idle minute; Claude's applied model and reasoning level are also read back from the running CLI. A newer runtime requires an agent restart.", "/model\n/model <id-from-catalog>"},
	{"Model settings", "effort", "Inspect or select reasoning strength", "Shows reasoning levels for the active model, including Claude's default model. Current shows the applied level; Model default (reset) clears the override and is separate from the reasoning choices. Claude changes are saved only after the CLI confirms the applied level. cxz restores the confirmed choice on resume, including max when the model supports it. Unsupported or unverified changes are rejected without creating a chat turn.", "/effort\n/effort high\n/effort default"},
	{"Conversation", "answer", "Open an interactive question dialog", "Questions open automatically below the conversation. F6 moves focus between the question and conversation; Ctrl+Q opens the session list while retaining answers. /answer reopens the selected pending question. Up/Down or Tab moves; Space/Enter selects or toggles options. Click an option or its description to select.\nOther accepts a custom answer.\nKeyboard focus and mouse hover highlight the full option width. Hover temporarily hides keyboard focus; leaving without clicking restores it, while clicking commits the new position. Next/Back navigates questions; Submit sends all answers together. These buttons stay visible while scrolling and accept clicks. Ctrl+S advances/submits. Esc/Cancel closes without denying. PgUp/PgDn or the wheel over the question scrolls its content. The wheel over the conversation or Ctrl/Alt+PgUp/PgDn scrolls conversation history without closing the question. /approval retains the raw payload. Codex asynchronous questions remain answerable after their turn ends; answers are sent as tool output, not approval RPC replies. Codex questions are single-select; Claude may request multiple selection. Advanced JSON replies are still accepted, but are not needed for the dialog.", "/answer\n/help answer"},
	{"Conversation", "context", "Inspect provider context", "Both providers use the same Summary view: token usage, window, available space and a utilization bar. Claude runs its native /context command and requires an idle session; Codex uses the last reported turn snapshot. Categories and token metrics appear only when reported. Click Summary/Raw or use Left/Right or Tab to inspect the original response. Missing data is not guessed.", "/context"},
	{"Conversation", "compact", "Compact agent context", "Requires an idle session. Runs native Claude /compact or Codex thread compaction. The provider reduces its conversation context; cxz keeps the full event journal. No arguments are accepted.", "/compact"},
	{"Permissions", "permission", "Choose manual or automatic approval", "ask saves manual approval. full saves automatic approval for this session: its supervisor handles supported pending and future tool requests even when another session is viewed or the TUI is closed. The policy survives restart/resume and is shown from server state. New sessions and sessions without a saved policy start in full mode; explicitly saved ask is preserved. Questions and unknown request types still need a reply. Use /permission ask to disable it; decisions already sent cannot be retracted. Provider sandbox settings are unchanged", "/permission full\n/permission ask"},
	{"Permissions", "approval", "Inspect the selected request", "Shows the full selected pending approval payload locally. Tab focuses Pending approvals; arrow keys select a request. Enter allows it and Backspace denies it. Use /answer for a question that needs a structured response.", "/approval"},
	{"Inspection", "usage", "View session usage and account quota", "Reads session tokens, cost and duration from the full journal. Account quota is separate provider telemetry, queried at initialization, after turns and every minute while active. This command reads recorded data, not a fresh provider request. It also explains waiting, timeout, unsupported, unavailable and error states.", "/usage"},
	{"Inspection", "download", "Download a container file", "Save a regular file from the selected project container to this client’s Downloads folder. Absolute, ~/ and project-relative paths work; path arguments offer Up/Down, Tab and Enter completion. Spaces can be entered directly or quoted. Existing files are preserved with numbered names. /download --cancel stops a transfer. Directories must be archived first. File bytes and the command are not sent to the agent.", "/download /workspace/report.zip"},
	{"Inspection", "view", "Select and inspect tool activity", "Select a rendered tool row with Up/Down, then Enter to open the same preview as a mouse double-click. Read and other results use recorded output; Write/Edit use submitted content/diffs. Home/End selects the first/last loaded tool. Up at the first tool loads older history when available. Enter focuses the dark-gray preview with 16 content rows above the input (shorter terminals reduce the height), or the full height of the right sidebar on wide screens, with side padding and a bottom focus rule; arrows/PgUp/PgDn/Home/End scroll it, x or Esc closes it, and Tab returns to selection. Esc in selection returns to the composer. Tool rows highlight on hover; a single click starts text selection and a double-click opens the preview. Mouse-opened previews also receive keyboard focus; Tab returns to input. The title-bar ⧉ button or Ctrl+C copies the full recorded content through the terminal clipboard (OSC 52). Pasted keys never trigger preview actions.", "/view"},
	{"Inspection", "details", "Inspect the latest tool result", "Shows the full latest tool result locally instead of its compact transcript summary. Use /view or double-click a tool row to open its shaded preview. Input/Output tabs show the recorded request and result. Left/Right or a tab click switches tabs. Write/Edit initially show highlighted input; other completed tools show output. Scroll with arrows or the mouse wheel; x, Esc or × closes the focused panel. Earlier raw events remain available through the cxz session events CLI command.", "/details"},
	{"Session", "stop", "Stop the current agent", "Stops the agent process, not just the current turn. Session history remains and Ctrl+R resumes a stopped session. To interrupt only the current turn, press Esc twice within three seconds; Ctrl+D instead detaches while the agent continues.", "/stop"},
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
			if strings.TrimLeft(entry.name, "/@") == strings.TrimLeft(topic, "/@") {
				return indentBlock(ansi.Hardwrap(lavender.Bold(true).Render("cxz /help "+topic)+"\n\n"+
					strong.Render(entry.summary)+"\n"+muted.Render(entry.detail)+"\n\n"+
					accent.Render("Examples")+"\n"+entry.example+"\n\n"+muted.Render("Submit with ")+helpKeycaps("Ctrl+S", 0), w, true))
			}
		}
		return indentBlock(ansi.Hardwrap("Unknown help topic: "+safeText(topic)+"\nUse /help to list commands; for example /help answer.", w, true))
	}
	profile := map[termenv.Profile]string{termenv.TrueColor: "24-bit True Color", termenv.ANSI256: "256 colors", termenv.ANSI: "16 colors", termenv.Ascii: "no color"}[lipgloss.ColorProfile()]
	lines := []string{lavender.Bold(true).Render("cxz /help"), muted.Render("Detected color profile: " + profile), muted.Render("Details and examples: /help <command> · e.g. /help answer")}
	lines = append(lines, muted.Render("Paths: backtick + / or ~ for container · backtick + ! for host attachments · Tab browses · Enter finishes"))
	category := ""
	for _, entry := range commandHelpEntries {
		if category != entry.category {
			category = entry.category
			lines = append(lines, "", accent.Bold(true).Render(category))
		}
		name := entry.name
		if name == "attach" {
			name = "`!path`"
		} else if !strings.HasPrefix(name, "@") {
			name = "/" + name
		}
		lines = append(lines, name+"  "+muted.Render(entry.summary))
	}
	shortcuts := []struct{ category, keys, text string }{
		{"Input", "Shift+arrows / mouse drag", "Select composer text; click to place cursor"},
		{"Input", "Ctrl+C / Ctrl+X", "Copy / cut selected composer text; Esc clears selection"},
		{"Input", "Ctrl+← / Ctrl+→", "Move by word (Alt+B/F also supported)"},
		{"Input", "Ctrl+Shift+← / →", "Select by word"},
		{"Input", "Shift+Home / End", "Select to the start/end of the visible row"},
		{"Input", "Home / End", "Start/end of the visible row; again steps to the next row"},
		{"Input", "Alt+W", "Toggle visible spaces (·) and newlines (↵); soft wraps keep a blank gutter"},
		{"Input", "Wheel over the composer", "Scroll draft without moving cursor or selection; typing returns to cursor"},
		{"Input", "Tab / Shift+Tab", "Indent/outdent; completions and visible approval/error focus take priority"},
		{"Input", "Alt+↑ / Alt+↓", "Move current line or selected logical lines"},
		{"Input", "Alt+D", "Duplicate selection or current line"},
		{"Input", "Double / triple click", "Select a word / logical line; drag to extend"},
		{"Input", "Ctrl+Z / Ctrl+Y", "Undo / redo editing"},
		{"Navigation", "Click the pinned prompt", "Scroll back to the message it names"},
		{"Navigation", "F9", "Start/stop debug recording and save"},
		{"Input", "Ctrl+S", "Send message or command"},
		{"Input", "Ctrl+Enter", "Send message or command (Windows console / Kitty protocol)"},
		{"Input", "Enter / Alt+Enter / Ctrl+J", "Newline with automatic indentation"},
		{"Input", "Ctrl+X", "Clear draft"},
		{"Inspection", "Ctrl+C", "Copy dragged conversation selection, focused tool content, or report (OSC 52)"},
		{"Navigation", "Tab / Shift+Tab", "Next / previous focus: approvals → input"},
		{"Navigation", "Alt+← / Alt+→", "Back/forward through visited sessions"},
		{"Navigation", "↑ / ↓", "Select project/session (panel focus) or slash hint"},
		{"Navigation", "PgUp / PgDn", "Scroll transcript (mouse wheel also works)"},
		{"Navigation", "Ctrl+End", "Follow latest output"},
		{"Navigation", "r", "Rename session in project panel; Enter saves, Esc cancels"},
		{"Navigation", "Ctrl+Q", "Focus project list: n new, a accounts, s stop"},
		{"Navigation", "Ctrl+X twice", "Delete selected session in project panel within 3s; stops agent, retains journal"},
		{"Navigation", "Ctrl+.", "Open settings · shared Docker status and maintenance"},
		{"Navigation", "Ctrl+N", "Create session; project title hover + opens the same picker"},
		{"Navigation", "?", "Open help from the project panel"},
		{"Navigation", "m", "Browse selected session memory in the project panel"},
		{"Approval controls", "↑ / ↓", "Select pending request (approval focus)"},
		{"Approval controls", "PgUp / PgDn", "Scroll full request (approval focus)"},
		{"Approval controls", "Enter / Backspace", "Allow / deny (approval focus)"},
		{"Navigation", "F2", "Edit project title; Enter saves, Esc cancels"},
		{"Agent controls", "Esc", "Press twice within 3s to interrupt current turn"},
		{"Agent controls", "F4", "Interrupt current turn immediately"},
		{"Agent controls", "Ctrl+R", "Resume stopped session; yellow dot blinks until the request finishes"},
		{"Agent controls", "Ctrl+D", "Detach; agent continues"},
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
