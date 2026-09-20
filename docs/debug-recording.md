# TUI debug recording

Press **F9**, or open **Ctrl+P** and select **Start debug recording**. Close
settings and reproduce the problem. Press **F9** again to stop and save a JSONL
file. `/record` also toggles recording; `/record status` shows the full saved
path in a scrollable report. Settings shows the last saved path too.

A `REC` indicator stays visible while recording. Normal TUI exit also saves an
active recording. If saving fails, the recording stays in memory; F9 retries.
Forced termination cannot save the in-memory buffer.

Files are stored in `recordings/` under the client state directory (`--state`),
normally `~/.local/state/cxz/recordings/`, respecting `XDG_STATE_HOME`. Files are
created with mode `0600`. Recordings stay local until you share them yourself.
Only the latest 12,000 events are retained; the first JSONL record reports the
number of older events dropped.

This is a diagnostic trace, not a screen video. While enabled, it records:

- Navigation escape sequences and their translation, plus sequence lengths for
  other CSI protocol events and incomplete-sequence timeouts.
- Decoded key categories, word-navigation bindings, focus, window dimensions,
  composer cursor positions and input length before and after updates.
- TUI event types, selected RPC error codes, update/render durations and rendered
  byte counts.
- OS, architecture, Go/build version, terminal identification and whether SSH or
  tmux is present. Sessions are represented by recording-local numbers.

It excludes typed or pasted text, conversation contents, rendered screen text,
raw error messages, account/project names and authentication data. Server and
agent debug log streams are not included. A recording is useful for diagnosing
whether a navigation key reached cxz, how it was decoded, and which view handled
it. Terminal or desktop shortcuts that intercept a key before cxz receives it
will not appear as key events.

Ctrl+Left/Right and Alt+Left/Right move by words in the composer; Alt+B/F are
alternatives. The client also translates Kitty primary navigation key codes.
These changes require updating the cxz client, without recreating the manager
or project containers.
