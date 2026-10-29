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

### Reaching the manager from another machine

Three links, and what each one proves:

| | Who is proven | Encrypted | Credential |
|---|---|---|---|
| `ssh://` | both, by ssh | yes | your ssh key and the host key |
| `mtls://` | both, by certificate | yes | a certificate this installation signed |
| `tcp://` | the client only | **no** | a bearer token, sent on every call |

The installation holds one self-signed root, created with the manager, kept in its
state volume and never replaced. It signs two things: the relay's own certificate,
and a certificate per client. Verification ignores names and checks the chain and
the extended key usage, so a client certificate cannot be presented as the relay
and an address change breaks nothing.

The relay that listens is a separate container. It is given the manager's socket
and its own leaf, both read-only, and never the root's private key — a process on
a network cannot issue credentials. Signing and revoking happen inside the
manager, on the host.

**Any valid client certificate is the installation's owner.** There is one
capability, not per-project scoping, exactly as with the TCP token. What a
certificate adds is that it expires (30 days) and can be refused individually:
`cxz expose revoke SERIAL` applies from the next handshake on the running relay,
without restarting it or disturbing other clients. A lost laptop is one revoke
rather than a token rotation for everyone.

Enrollment over ssh carries no secret: the key is generated on the client and
stays there, and only a certificate request and a certificate cross the channel.

## Journals contain your source

Raw journals hold everything the agent saw and did, including file contents it read
and commands it ran. They can contain secrets your code contains.

**Do not publish them.** That includes pasting them into issues. `/details` and the
transcript show abbreviated views; `cxz session events` does not.

## /@redact

Pasting a complete recognized GitHub token or signed JWT into the conversation
composer automatically inserts a Secret chip, including short tokens that would
otherwise remain inline. Detection runs locally before draft persistence or
ordinary attachment upload. Only surrounding whitespace is ignored for matching;
the secret file preserves the original pasted bytes. Text containing a token
inside prose, code, JSON or an environment assignment is not detected. Question
answer fields and file uploads retain their existing behavior.

The browser and TUI share `internal/secretpaste/patterns.json`. Add portable
regular expressions there; both clients require a whole-string match. GitHub
rules check the documented prefixes, token character sets and a minimum suffix
length rather than pinning a total length. JWT recognition also decodes the
header and payload as JSON objects and requires a nonempty string `alg` header.
It covers Kubernetes ServiceAccount tokens without relying on the incidental
`eyJhbG` prefix. Recognition does not verify signatures, expiry or credentials
against a remote service. Format-matching expired or example tokens are still
treated as secrets.

Browser preparation immediately inserts a safe placeholder and blocks Send until
the secret file is ready. The chip uses a fixed six-character slot, showing a
CompactNumber percentage such as `  099%` while preparing and `Secret` afterward.
Progress is capped at 99% until the server acknowledges storage. It uses the
same authenticated Connect RPC with XHR for upload progress. Ready chips show
only `Secret`; internal IDs remain internal. Failure, cancellation or an
unavailable session never
falls back to a normal upload or plaintext draft: remove the failed chip and
paste again. Refresh interrupts pending preparation; a ready file reference can
be restored for the same run. The existing 64 KiB secret limit and session/run
binding apply. TUI secrets are kept in memory until Send, like `/@redact`.

Opening a browser Secret card reads the temporary file through the existing
project Download API, with a 64 KiB limit and the same session/run binding.
The body contains only its value, blurred until dragged/selected. Clearing the
selection or losing window focus conceals it again. Blur is a visual privacy
affordance; the open card holds its plaintext in memory. Closing the card aborts
pending reads and drops its transient value; previewing does not persist it in
drafts or ordinary attachment storage. Expired files show an error.

The card's Expand button requires a second confirmation click. Confirming
explicitly converts that occurrence into ordinary editable text: the raw value
then enters the draft and, if sent, the prompt and conversation record. Closing
the card or canceling confirmation keeps the file reference. An edited draft
cannot be overwritten by a stale preview.

Format references:

- [GitHub token formats](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github#githubs-token-formats)
- [Kubernetes ServiceAccount tokens](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#service-account-tokens)
- [JWT format](https://www.rfc-editor.org/rfc/rfc7519.html#section-3)

`/@redact` keeps a secret out of the journal. You reference a value, the helper
writes it to a temporary file inside the container with the remote user's
permissions, and the journal records the reference — not the value.

The temporary file's lifetime is independent of your connection: it is cleaned up on
helper start and by explicit deletion, not when the TUI closes. It lives inside the
project container, readable by the project's remote user, which means **anything
running as that user in that project can read it** — including the agent.

`/@redact` protects the durable record. It does not protect the running container.

### Which connection may carry one

The helper that writes the file runs in the project container, reached through
the engine. A client on the daemon host starts it itself, so the secret never
goes near a network. A client connected over `ssh://` cannot, so the manager is
asked to write the file instead — the secret crosses the ssh connection, which
encrypts it.

A client connected over `mtls://` is in the same position as an ssh one: the
link is TLS to a peer whose certificate this installation signed, so the manager
is asked to write the file and the secret is encrypted on the way.

Over the exposed plaintext TCP surface it is refused. That link is authenticated
**plaintext**, intended for a tunnel, and cannot be told apart from a local
client on the daemon side, so the refusal is the client's: connect over `ssh://`
or `mtls://`, or put a tunnel under the TCP endpoint and use it for the shell as
well.

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
