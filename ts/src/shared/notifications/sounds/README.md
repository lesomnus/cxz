These original PCM notification sounds match `internal/notification/sounds`.
Keep the web copies synchronized when the TUI cues change. Vite bundles them as
same-origin assets; no external audio service or notification permission is used.

The web-only error.wav is an original 24 kHz mono PCM descending two-note cue
(660 Hz, then 440 Hz), with short attack and quadratic decay envelopes. It is
distinct from completion and question cues and takes priority over queued cues.
