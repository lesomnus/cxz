# `@redact`: host-tmpfs secret files

Type `@` at the start of input or after whitespace to show inline commands.
For example, `foo @redact` + Enter opens hidden input. Tab completes the command;
Esc dismisses. Enter in the dialog replaces only the command with `[Redacted]`,
preserving surrounding text. Ctrl+X clears; Esc restores the draft/cursor.
Emails, backtick literals and newly pasted text do not activate the list.
Normal Enter remains newline; Ctrl+S sends. `/help redact` describes the feature.

On send, cxz transfers the secret directly to wisp over Docker exec stdin, not
the manager API. It sends only this replacement to the conversation/journal:

```
(secret placed at /cxz/secrets/1a2b3c4d/value)
```

The random directory name is 8 hexadecimal characters; exclusive creation retries
collisions. Directories are 0700 and files 0600, owned by the project remote user.
Limits are 64 KiB per secret and 16 unsent chips. Secret chips cannot be converted
to normal paste text, previewed or attached. Unsent values remain only in TUI
memory and are lost on exit. Go/terminal buffers prevent guaranteed memory erasure.

## Storage and lifetime

- Docker engine host source: `/dev/shm/cxz-<installation-owner>/<project-id>`.
  Projects bind it to `/cxz/secrets`; the manager binds the installation root to
  `/cxz/host-secrets`. The real filesystem must be writable tmpfs. Host
  nodev/nosuid/noexec flags are not required; their defaults vary across systems.
  No disk fallback or host mount-namespace modification.
- Files survive TUI, agent and manager exit, and project container stop/removal/
  recreation, while the same installation/project identity is retained.
- The old 15-minute creation TTL and disconnect/session-stop deletion are removed.
- The manager sweeps at startup and hourly, including orphaned project directories.
  Wisp also sweeps its project directory when started. Files older than **8 hours**
  by the later of atime/mtime are deleted. Directory metadata checks do not read
  the secret contents. No strictatime requirement: atime is only an approximate
  usage signal under relatime/noatime, so actively used files can still expire.
- With everything stopped, no sweep runs. Old files are removed on next startup.
  The Docker engine host's OS reboot clears its tmpfs. With Docker Desktop this
  refers to the Linux engine VM, not the client OS. tmpfs can still swap.
- Failed writes/submission attempts clean up their allocated files; accepted sends
  retain them. A lost send reply can mean the message was accepted despite cleanup.
- Cleanup only visits cxz's 8-hex-character directories and regular `value` files,
  without following symlinks or recursively removing unrelated contents.

## Upgrade

Update/recreate the manager to add the host bind mount, then recreate project
containers to replace their old per-container tmpfs mount with the host bind.
Old wisp retention policies are refused before sending new secrets. Preserve
important writable-layer data before container recreation. Existing files in the
old per-container tmpfs are not automatically migrated and disappear with it.
After host reboot, manager startup restores empty known project directories.
Uninstall/purge or changing installation/project identity is not a retention promise.
The host tmpfs shares the engine host's `/dev/shm` capacity; the former per-project
1 MiB mount limit no longer applies.

## Trust boundary and validation

This prevents accidental raw-secret inclusion in prompts, not access by the agent,
same-UID sessions, container root or Docker host. Reading/printing secrets can put
them in provider context and logs. Prefer tools consuming the file without output.
Host swap policy, core dumps, clipboard history and recordings are outside scope.

Tests cover input privacy, path-only replacement, failed-send cleanup, policy
negotiation, filesystem/symlink checks, idle timestamps and survival on helper exit.
The opt-in Docker test stops the original container and checks the same host-tmpfs
file from a newly created container, then cleans up its isolated fixture:

```
CXZ_TERMINAL_DOCKER_TEST=1 go test ./internal/containerterm -run '^TestDockerWisp$' -v
```

Docker also supports tmpfs-backed named volumes with its local driver options:
[docker volume create](https://docs.docker.com/reference/cli/docker/volume/create/).
This implementation deliberately binds an independently existing host tmpfs rather
than relying on the lifetime of a Docker-managed tmpfs mount.
