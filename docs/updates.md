# Updates and versions

cxz has several pieces that can be at different versions: the **frontend** you
typed into, the **manager**, each **project runtime**, and each session's
**supervisor** and agent. They update independently and in that order.

## Channels

```sh
cxz use @edge      # latest main build that passed CI
cxz use @stable    # latest published release
cxz use v0.1.2     # install and pin that version
cxz use            # what is running, which channel, pinned or not (JSON)
cxz use --unpin    # release the pin; nothing restarts
```

`@stable` picks the highest release version, excluding drafts and prereleases.
`@edge` uses a list published only after all four platform binaries and a
digest-pinned manager image exist for the same commit. Branch names are not
accepted.

`cxz use` verifies checksum, platform and build revision before replacing anything.
It needs no Go and no source build tools.

**Choosing a channel explicitly is immediate and forceful.** On a Linux host it
replaces and restarts the frontend, the manager, running project runtimes and
running session supervisors and agents, plus an installed running web gateway.
It does not wait for idle, so **work in progress is interrupted**. Conversations reopen with their history in a new run and
the last input is not resent. Sessions already stopped stay stopped. Project
containers and data volumes are not recreated. It does not change the provider CLI
versions.

Explicit switching allows downgrades. Automatic updating does not.

## Automatic updates

Unmodified builds descended from published `main` update themselves on the channel
you selected. Checks are daily; the apply queue re-evaluates safety every 30
seconds. Modified builds and builds that are not ancestors of published `main` are
never replaced automatically.

```sh
cxz self-update status      # frontend policy and state
cxz self-update check       # re-read the manifest; does not restart
cxz self-update disable
cxz self-update enable
```

Add `--server` **on the Linux host** for the manager's own policy. Attaching a
remote frontend does not change server policy, and a frontend update does not carry
a server-side fix.

What waits for what:

| | Replaced when |
|---|---|
| **Manager** | Every running session in the installation is safely idle |
| **Project runtime** | That project's sessions are safely idle, or an explicit `project recreate` |
| **Supervisor / agent** | That session has been safely idle for five minutes |
| **TUI** | Five minutes without input, and no draft, modal, attachment or in-flight request |

Preserved across each: project containers, state volumes, session and run identity
(a supervisor replacement makes a new run), account and model, and the journal. The
TUI restores its connection, project, session and scroll position.

A stopped session is never started just to update it; it picks up the new
supervisor on the next explicit resume.

Active work always wins. Tools running, background tasks, child processes, pending
approvals, a settings change in flight, or simply not being able to tell — all of
them defer the replacement.

## Building from source

On **Linux**, `cxz self-update` fetches `main` and builds it in Docker, then
replaces the local executable and the installed manager. `--ref` takes a branch, tag
or commit; `--client-only` skips the manager. Docker Buildx and a Linux builder are
required; host Git and Go are not. Builds are verified before replacement and the
previous executable is kept. Running sessions continue. An installed, running
web container is then recreated using the updated Manager image; its saved web
configuration is preserved. Browser users sign in again, but agents continue.
Stopped or absent web containers stay stopped/absent. See [web setup](web.md).

`self-update` checks the **installed manager's own revision** before reporting
success, by asking the container binary rather than trusting the host CLI. An
installer that reused a cached image cannot report a successful update.

Protected locations such as `/usr/local/bin` are handled by a Docker root installer,
which preserves ownership and permissions after confirming Docker sees the same
directory.

On **Windows**, `cxz self-update` downloads the latest tested `main` build from the
`edge` release. No Docker, Git or Go. Checksum, platform, build information and
version are checked before replacing `cxz.exe`, and `cxz.previous.exe` keeps the old
one. `--ref vX.Y.Z` takes a published tag.

## Rebuilding the manager image

On `@edge` or `@stable`, `install --recreate` builds the manager image with the
**current** CLI rather than reusing the image from when the channel was selected. An
explicit `--image`, and a pinned release, keep the image they name.

That distinction matters after a `self-update`: without it, refreshing the manager
could quietly reinstall the version you were trying to leave.

## Pinning

A pin blocks `self-update` and blocks `install` with a different image. Automatic
updating respects it. `--unpin` restores the channel and policy you had before.

Selecting a channel clears a pin, so you do not need to unpin first. To install a
manager for the first time while the client is pinned: unpin, install, pin again.

## Recovering a failed switch

Files and images are prepared first; only then are requests blocked and components
replaced. The resolved plan is written to `use-plan.json`, and completed steps to
`use-transaction.json` and each project's `use-project.json`.

**Re-run the same command.** It completes the build it originally chose, even if a
newer one has since appeared, and does not restart components that already finished.
While a transaction is incomplete, choosing a different version or unpinning is
refused. The previous manager container is left stopped as
`-before-use-<transaction>`.

A target that cannot read the current database version is refused before anything
stops. Releases that change data formats need their own compatibility check.

A TUI sharing the same client state notices the switch and restarts itself; unsaved
drafts are not preserved. Older frontends without that support must be reopened by
hand. Clients on other machines, and hosts reached over SSH, each run `cxz use`
themselves.

## Diagnostics

```sh
cxz version
cxz use
cxz self-update status [--server]
cxz manager doctor
cxz project logs PROJECT
```
