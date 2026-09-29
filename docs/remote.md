# Remote access

The containers live on a Linux host. The TUI does not have to.

```sh
cxz --endpoint ssh://user@host          # over SSH
cxz --endpoint tcp://host:7349 --token-file FILE
cxz --endpoint work                     # a named connection
cxz --endpoint local://                 # the local installation
```

A remote frontend is the same TUI against someone else's manager. It needs neither
Docker nor Go — a Windows client is a single executable.

## Named connections

```jsonc
// settings.jsonc
"connections": {
  "default": "work",
  "entries": {
    "work": { "target": "ssh://me@build-host" },
    "home": { "target": "tcp://192.168.1.10:7349", "tokenFile": "~/.cxz-home-token" },
  },
}
```

With connections configured, plain `cxz` shows **every** connection's projects and
sessions in one panel, labelled `project1 via work`. `default` decides the initial
focus. `cxz --endpoint work` focuses that one; giving a URL directly opens only it.

Use `local://` alongside remote targets to keep your own installation in the list.

## TCP forwarding

SSH needs no server-side setup. For TCP, open a relay on the host:

```sh
cxz expose --token-file FILE --listen tcp://127.0.0.1:7349
```

`expose` is an independent relay — it does not require restarting the manager or
reinstalling.

**TCP is authenticated, not encrypted.** The token proves who you are; it does not
hide what you send. Run it over a VPN or an SSH tunnel, never across the open
internet. Binding to `127.0.0.1` and tunnelling is the safe default.

## What stays on the host

Some things are host-local by nature and are unavailable or reduced over a remote
connection:

- **Host file attachments** come from the machine running the *client*, and are
  uploaded to the daemon.
- **Container path completion** runs on the host through a project helper, reached
  by an RPC — the client's own filesystem is never browsed.
- **Server settings and the manager's own update policy** are managed on the Linux
  host. Attaching a frontend does not change them; `--server` on the host does.

## Windows

The Windows frontend provides the TUI, `cxz edit` for local settings and model
preferences, `cxz self-update`, and connection management. It uses `VISUAL`,
`EDITOR`, then Notepad.

It reads console modifier flags directly, so `Ctrl+Enter` works without the
terminal-capability guesswork Unix needs, and it decodes console records without
passing Unicode through the system code page.

Host-side settings still apply on Linux. A remote manager updates separately, on its
own host.
