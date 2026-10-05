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
{
  "connections": {
    "default": "work",
    "work": { "target": "ssh://me@build-host" },
    "home": { "target": "tcp://192.168.1.10:7349", "token_file": "home-token" }
  }
}
```

With connections configured, plain `cxz` shows **every** connection's projects and
sessions in one panel, labelled `project1 via work`. `default` decides the initial
focus. `cxz --endpoint work` focuses that one; giving a URL directly opens only it.

Use `local://` alongside remote targets to keep your own installation in the list.

## Inspect connections and tunnel a port

These commands run on Windows and Linux clients:

```sh
cxz connection ls
cxz connection ls --format json
cxz connection tunnel work
cxz connection tunnel --local-port 7443 --remote-port 7350 work
```

`ls` reads local `settings.jsonc`, showing sorted names, configured targets,
the effective default and whether SSH tunnelling is supported. It does not
connect to any server, read bearer-token files or display credentials. Use
`cxz edit` to add or change connections. With no explicit default, the first
name alphabetically is the effective default, matching the TUI.

`tunnel` requires a saved `ssh://` connection and the system OpenSSH client
(`ssh.exe` on Windows). It forwards `127.0.0.1:7350` on your client to
`127.0.0.1:7350` on that SSH host by default. The optional port flags must precede
the connection name. Other connection types are rejected. The SSH endpoint's
username, SSH port and host alias are reused, including ordinary SSH config
identity files/ProxyJump. Its remote cxz `binary` and `state` query parameters
are irrelevant to port forwarding; no remote command is run.

The process remains in the foreground: keep the terminal open, and use Ctrl+C
to close the tunnel. Closing it leaves remote agents and the web server running.
SSH authentication/host-key errors and occupied local ports are reported by SSH.
Like the existing SSH connection, it uses batch authentication: configure a key
or ssh-agent and verify the host using ordinary `ssh` first. No passwords are
stored or prompted for by cxz. Existing multiplexed SSH sessions are not used,
so closing this command closes its own forwarding process.

The opening message means a connection attempt, not proof that the remote web
server is running. SSH tests the destination when a browser opens the forwarded
port; a stopped web server still produces a connection failure then.

### Browsing through the tunnel

A tunnel forwards bytes; it does not terminate TLS or rewrite HTTP Host/Origin.
The browser must therefore ask for the exact origin the remote gateway is
configured with.

A gateway left at its default makes that automatic. `cxz web up` serves
`http://127.0.0.1:7350`, the tunnel's own default forwards `127.0.0.1:7350` to
`127.0.0.1:7350`, and both ends spell the origin the same way, so
`http://127.0.0.1:7350` in the browser matches with nothing to configure and no
certificate to trust. SSH encrypts the hop.

Two things still break the match:

- **A different local port.** `--local-port 7443` makes the browser's origin
  `127.0.0.1:7443`, which the gateway refuses. Change the gateway's `origin` and
  `listen` to the same port, or keep the ports equal.
- **`localhost` instead of `127.0.0.1`.** The tunnel binds IPv4 loopback only,
  and browsers try `::1` for `localhost` first. Use the IP literal.

For an HTTPS gateway, the remote `origin` must be the URL you type and its
certificate must cover that name, so browsing `https://localhost:7350` needs
`origin: https://localhost:7350` and a certificate for `localhost` that the
client trusts. Keeping a VPN hostname and certificate instead means mapping that
hostname to `127.0.0.1` on the client and tunnelling on the same port. Choose
one arrangement consistently; cxz does not change certificates, hosts files or
web configuration automatically. Tunnel setup does not start the remote gateway
or bypass its browser token login.

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
