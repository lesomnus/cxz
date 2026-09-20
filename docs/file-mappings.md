# Host file mappings

Copy host files or directory contents into agent containers. Mappings are global
within one cxz installation, with an optional `claude` or `codex` filter.

Open the configuration directly in your editor:

```sh
cxz edit
```

This opens `<state>/settings.jsonc` (normally `~/.local/state/cxz/settings.jsonc`)
using `$VISUAL`, then `$EDITOR`, or an installed `nano`/`vim`/`vi`. Editor arguments
such as `code --wait` are supported. New files start with commented examples;
line/block comments and trailing commas are accepted. For example:

```json
{
  "claude_model": "sonnet",
  "files": [
    {
      "src": "~/instructions/CLAUDE.md",
      "dst": "${AGENT_CONFIG_DIR}/CLAUDE.md",
      "agent": "claude"
    },
    {
      "src": "~/instructions/AGENTS.md",
      "dst": "${AGENT_CONFIG_DIR}/AGENTS.md",
      "agent": "codex"
    }
  ]
}
```

Save and exit to validate the configuration. Changed mappings are snapshotted
and published automatically; model-only changes are saved without contacting the
server. Unchanged edits do nothing. Invalid edits, editor failures, and concurrent
configuration changes preserve the original and report the retained draft path.
A publication failure leaves valid local settings saved for a later sync.

`cxz config files ls`, `add`, and `remove` remain available for scripts. For example:

```sh
cxz config files add --agent claude ~/instructions/CLAUDE.md '${AGENT_CONFIG_DIR}/CLAUDE.md'
```

Quote destinations containing variables so your shell does not expand them.

| Destination prefix | Resolved inside the container |
| --- | --- |
| `${AGENT_CONFIG_DIR}` | This session's Claude configuration directory or Codex home |
| `${SESSION_HOME}` | This session's isolated `HOME` |
| `${WORKSPACE}` | The session's container workspace directory |
| `/absolute/path` | An explicit container path, writable by the agent's OS user |

`src` is a path on the machine running the command, not on the Docker engine.
Relative paths are resolved when registered; `~/` is supported. A source file
copies to the exact `dst` path. A source directory copies its contents under
`dst`, retaining relative names; a directory can target `${AGENT_CONFIG_DIR}`
itself. Empty directories are not copied. Only regular
files are supported; source symlinks and symlink destination parents are rejected.
The bundle is limited to 512 files and 2 MiB of file contents. Destination files
are private to the container user (0600, or 0700 for executable source files).

`add` replaces the mapping with the same destination and agent filter, then
publishes a snapshot of all registered sources. To publish later source edits:

```sh
cxz config files sync
```

The manager stores the snapshot durably and forwards it to running project
runtimes. Stopped projects receive it when opened. Before each agent process
starts (new session, resume, or agent restart), its supervisor copies the latest
received snapshot, substituting the session-specific paths. No open TUI or
connection to the source machine is required at that point. Running agents are
not restarted and their files are not changed by sync. For an absolute or
workspace destination shared by several sessions, another session starting can
refresh that shared file.

Source changes are **not watched automatically**. Removing the source after a
successful sync does not remove the stored snapshot. All configuration/file
mapping commands use the host's local `settings.jsonc`; publishing replaces the
installation's complete mapping bundle. A second client should use the same
mapping configuration before syncing.

To stop copying a mapping:

```sh
cxz config files remove --agent claude '${AGENT_CONFIG_DIR}/CLAUDE.md'
```

Files already copied into sessions are retained, including files removed from a
source directory. Configured destination files are overwritten at agent startup;
edit the host source, not the destination, to keep changes across restarts.

If publishing fails, local configuration remains saved and the command reports
an error; retry `cxz config files sync`. If only some project runtimes reject the
update, the manager retains the new bundle and reports those projects. New and
resumed sessions must receive it successfully before starting.

This feature requires an updated host client, manager, and project runtime.
Updating the binaries does not restart existing agents automatically.
