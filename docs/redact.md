# `@redact`: temporary secret files

Type `@` at the start of the composer or after whitespace to show inline commands.
For example, type `foo @redact`, then press Enter to open hidden input. Tab
completes the command name; Esc dismisses the list. The hidden-input overlay accepts typing or pasted
text (including pasted newlines), shows only a character count, and never previews
the value. Enter replaces only `@redact` with `[Redacted]` in the composer;
surrounding text is preserved and the cursor moves just after the chip. Subsequent
chips have numbered labels. Ctrl+X clears the hidden input, Esc restores the
original draft and cursor. Email addresses, backtick literals and newly pasted
text do not automatically activate the list. Normal Enter remains newline;
Ctrl+S sends the message. `/redact` is no longer advertised as a slash command.
The chip is atomic:
delete it normally or select it and press `d`. `t`, `f`, and paste preview cannot
convert or reveal a secret chip. `/help redact` describes the feature in the TUI.

On composer submission, cxz sends the secret directly to wisp over its Docker
exec stdin connection, not the manager API. Wisp creates a random private
directory and a `value` file. cxz replaces the chip in one pass with:

```
(secret placed at /cxz/secrets/<random-directory>/value)
```

Only that replacement reaches the conversation API/journal. Ordinary paste
contents are not recursively interpreted as secret chips. Each chip is bound to
its original session/run. Limits: 64 KiB per secret and 16 unsent secret chips.
Hidden input and pending bytes live only in the TUI process, not saved drafts,
attachments or account storage. Secret buffers are cleared on cancellation,
deletion, successful send, failed send and normal TUI exit where feasible; Go's
allocator, terminal input buffers and copies prevent guaranteed memory erasure.

## Container setup

New image/Dockerfile and Compose projects mount a dedicated 1 MiB tmpfs at
`/cxz/secrets`, with `nosuid,nodev,noexec` and root directory mode `1777`. Each
random child directory is `0700`, and each secret file is `0600`, owned by the
project remote user. Wisp checks filesystem type and mount flags before receiving
a secret from the client and again before creating it. Directory-relative file
descriptors, exclusive creation and no-follow flags avoid following a substituted
leaf symlink. Cleanup touches only files allocated by that helper.

Existing containers require recreation to acquire the mount; updating the binary
or restarting the agent cannot add it. Old wisp versions are rejected before
secret transfer. Missing or incorrectly configured tmpfs is an error, never a
disk-backed fallback. Update manager/runtime before recreating. Container
recreation removes its writable layer: preserve important files outside workspace
mounts first. cxz does not recreate containers automatically for `@redact`.

## Lifetime and limits of protection

- Files expire 15 minutes after creation while wisp is running. They are **not**
  deleted at turn completion, because background tools may still need them.
- Failed transmission attempts clean up allocated files and never automatically
  retry the conversation. Enter the secret again after failure. A lost send reply
  can still mean the agent accepted the message; its file may already be removed.
- Observed stopped/failed sessions or changed run IDs trigger scoped cleanup.
  Normal helper stdin EOF (including a tested TUI connection close) removes all
  files allocated by that helper. Files do not persist across normal detach.
- A killed/frozen helper cannot enforce its timer. Stop the project container to
  discard its tmpfs if cleanup cannot run. Unlinking also cannot revoke copies or
  descriptors already held by another process.
- This avoids accidental secret inclusion in prompts; **it does not isolate the
  secret from the agent, other sessions running as the same UID, container root,
  or the Docker host**. An agent reading/printing the value can put it in tool
  output, provider context and logs. Prefer tools consuming the file without
  displaying its contents; this feature cannot enforce that behavior.
- tmpfs may swap. Host swap encryption/disablement, core-dump policy, terminal
  recording, clipboard history and provider handling are outside this feature.
- Current wisp transport needs Docker access to the manager's engine, like path
  hints and the embedded terminal; this is not a remote-only secret RPC API.

Tests cover masked input, no paste expansion/preview, one-pass path replacement,
send failure cleanup, old-runtime refusal before transfer, tmpfs/symlink checks,
TTL and session-scoped deletion. The opt-in Docker fixture verifies non-root
creation, contents, permissions, explicit deletion and connection-EOF cleanup:

```
CXZ_TERMINAL_DOCKER_TEST=1 go test ./internal/containerterm -run '^TestDockerWisp$' -v
```
