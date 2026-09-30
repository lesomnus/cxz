# Security and boundaries

What cxz does and does not protect you from. Read this before pointing it at
something sensitive.

## What cxz is not

**Not a sandbox for the agent.** The agent runs in a devcontainer cxz owns, with
your workspace mounted writable. That is isolation from your host, not isolation
from your code. Claude runs with its default tool set; Codex runs with its
`untrusted` approval policy and no nested sandbox.

**Not protection from hostile configuration.** `--trust-config` and the inline
**Trust** choice mean you have read the devcontainer configuration. An
`initializeCommand` runs on your host with your privileges.

**Not exactly-once.** A crash can leave a tool call in `delivery_unknown`. Check the
history before repeating a task.

## Credential boundaries

- Each session has its own agent configuration, HOME, cache and temporary
  directory. Refresh tokens are **never** copied between sessions or projects.
- Host credentials are never imported implicitly. A session without a login fails
  rather than falling back to environment API keys.
- No manager socket and no credential directory is mounted inside a project
  container. An agent cannot reach your other projects or accounts.
- No host port is published. The path is client → the manager's private socket over
  Docker exec stdio → the project network with a capability → a scoped runtime → the
  supervisor.
- Project TCP is authenticated, not encrypted. The Docker operator is trusted.
- A capability is a secret proving access to a specific project or account. Public
  IDs and aliases are not capabilities.

## Journals contain your source

Raw journals hold everything the agent saw and did, including file contents it read
and commands it ran. They can contain secrets your code contains.

**Do not publish them.** That includes pasting them into issues. `/details` and the
transcript show abbreviated views; `cxz session events` does not.

## @redact

`@redact` keeps a secret out of the journal. You reference a value, the helper
writes it to a temporary file inside the container with the remote user's
permissions, and the journal records the reference — not the value.

The temporary file's lifetime is independent of your connection: it is cleaned up on
helper start and by explicit deletion, not when the TUI closes. It lives inside the
project container, readable by the project's remote user, which means **anything
running as that user in that project can read it** — including the agent.

`@redact` protects the durable record. It does not protect the running container.

## Diagnostic recordings

`F9` recordings contain sanitized navigation sequences, decoded key categories,
focus and cursor changes, event types and render timings. They **exclude** typed and
pasted text, conversation content, credentials and raw screen output.

They are written locally; `/record status` shows the path. Review one before sharing
it — the exclusions are deliberate, but the file is still about your session.

## Deleting things

`cxz uninstall` removes the manager only. Projects, volumes, conversations and
credentials stay.

Deleting a session stops listing it and keeps its journal. `cxz session purge`
destroys one session for real: journal, agent profile and credentials, uploads,
published memories and the record with its title. It runs from the host client only
— an agent inside a project cannot purge the history it can read.

`cxz purge` is the installation-wide one, and deletes every conversation and
credential. Neither revokes tokens at the provider — do that yourself. See
[projects and sessions](projects.md#deleting-a-session).

## Backups

Back up the state volumes **and** your workspace. SQLite alone is not a backup: it
holds projections built from the journal, plus resource metadata and audit history
that cannot be rebuilt from journals. Back up all of it.
