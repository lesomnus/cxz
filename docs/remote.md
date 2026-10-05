# Remote access

The containers live on a Linux host. The TUI does not have to.

```sh
cxz --endpoint ssh://user@host          # over SSH
cxz --endpoint mtls://host:7349         # direct, with an enrolled certificate
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
    "lan": { "target": "mtls://192.168.1.10:7349" },
    "home": { "target": "tcp://192.168.1.10:7349", "token_file": "home-token" }
  }
}
```

With connections configured, plain `cxz` shows **every** connection's projects and
sessions in one panel, labelled `project1 via work`. `default` decides the initial
focus. `cxz --endpoint work` focuses that one; giving a URL directly opens only it.

Use `local://` alongside remote targets to keep your own installation in the list.

## Mutual TLS on your own network

SSH works everywhere and needs no setup on the host, but it starts a process per
connection and carries every conversation through a shell. On a network you
control, `mtls://` is a direct TLS connection to the same manager: the whole RPC
surface, authenticated both ways by certificates this installation issued.

The installation has one self-signed root, created with the manager and never
replaced. Clients pin it; the relay proves itself with a leaf it signed. **No
hostname is involved** — verification checks that a peer chains to that root and
carries the right extended key usage, so a DHCP lease that changes your host's
address breaks nothing, and a client certificate cannot be presented as the
relay.

### On the host: run a relay

The manager listens on a unix socket inside its container and nothing else, so
something has to put it on a network:

```sh
cxz expose up                 # background relay container, mtls://0.0.0.0:7349
cxz expose status             # listener, root fingerprint, enrolled clients, version skew
cxz expose down               # remove it; the root and issued certificates stay
```

`expose up` creates `<manager-container>-remote` from the manager's image with
Docker's `unless-stopped` policy, so it survives a reboot, and a version switch
refreshes it the way it refreshes the web gateway. It mounts the manager's socket
and its own certificate read-only, and **not** the root's private key: the
process on the network cannot issue anything.

Binding every interface is the point here, unlike the loopback web gateway: a
connection that cannot present a certificate gets no further than the handshake.

### On the client: enroll once

With an SSH connection to the same host, there is nothing to carry:

```sh
cxz connection enroll work
cxz connection ls             # work · ssh://me@build-host · mtls (expires 2026-11-04)
cxz connection check work     # dial it now and say what answered
```

SSH is the trust anchor, not the transport: it has already verified the host key
and this user, so a certificate handed back through it was issued by the
installation you meant to ask. From then on the connection is mutual TLS, and the
`TRANSPORT` column says so while `TARGET` still shows what you configured.

**Nothing is sent that would matter if it leaked.** The key is generated on the
client and stays there; what crosses the channel is a certificate request and a
certificate, both public. If the relay is not running, enrollment starts it over
the same channel and says it did — `--no-start` refuses instead.

A connection whose relay has stopped is repaired the same way on the next launch,
and an expired certificate is renewed. `cxz --no-enroll` turns both off for one
invocation, and falls back to ssh rather than failing.

### Without SSH

Move two public files instead. The key never travels:

```sh
# client
cxz connection enroll --csr lan > lan.csr
# host, with lan.csr copied over
cxz expose ca > ca.crt
cxz expose sign --label lan-desktop < lan.csr > lan.crt
# client, with lan.crt and ca.crt copied back
cxz connection enroll --certificate lan.crt --ca ca.crt lan
```

The connection's target supplies the address (`mtls://10.0.0.5:7349`); pass
`--address` when it does not. Compare `cxz expose status`'s root fingerprint with
what you pinned if the files travelled somewhere you do not trust.

### Revoking

A client certificate is a bearer credential: whoever holds it holds the
installation, exactly as the TCP token does. Two things make that bearable —
certificates expire after 30 days, and they can be refused one at a time:

```sh
cxz expose status --format json        # serials, labels and expiry
cxz expose revoke 7231...              # on the host
cxz connection forget lan              # on the client, deleting the local copy
```

Revoking takes effect on the **running** relay from the next connection: the list
is read per handshake, so nothing restarts and no other client is disturbed. A
first version has one capability — any valid certificate is the installation
owner — so revocation, not scoping, is what limits a credential you no longer
trust.

Mutual TLS is also a confidential link, so secret attachments (`@redact`) work
over it, which they do not over plaintext TCP.

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

## Plaintext TCP forwarding

SSH needs no server-side setup. For TCP, open a relay on the host:

```sh
cxz expose --token-file FILE --listen tcp://127.0.0.1:7349
```

Bare `expose` is an independent foreground relay — it does not require restarting
the manager or reinstalling, and Ctrl+C ends the exposure.

**TCP is authenticated, not encrypted.** The token proves who you are; it does not
hide what you send — it is sent on every call, so the credential is exposed along
with the conversation. Run it over a VPN or an SSH tunnel, never across the open
internet. Binding to `127.0.0.1` and tunnelling is the safe default, and
[mutual TLS](#mutual-tls-on-your-own-network) is the better one on a network you
control.

Both relays reach the same surface: a method-agnostic passthrough, so every RPC
and every stream behaves as it does locally. What differs is the credential and
whether the bytes are encrypted.

## What stays on the host

Some things are host-local by nature and are unavailable or reduced over a remote
connection:

- **Host file attachments** come from the machine running the *client*, and are
  uploaded to the daemon.
- **Container path completion** runs on the host through a project helper, reached
  by an RPC — the client's own filesystem is never browsed.
- **Server settings and the manager's own update policy** are managed on the Linux
  host. Attaching a frontend does not change them; `--server` on the host does.
- **The installation root** is created with the manager and lives in its state
  volume. Signing, revoking and `expose up` all run on that host, where Docker
  and the root are; a client only ever holds a certificate it was issued.

## Windows

The Windows frontend provides the TUI, `cxz edit` for local settings and model
preferences, `cxz self-update`, and connection management. It uses `VISUAL`,
`EDITOR`, then Notepad.

It reads console modifier flags directly, so `Ctrl+Enter` works without the
terminal-capability guesswork Unix needs, and it decodes console records without
passing Unicode through the system code page.

Host-side settings still apply on Linux. A remote manager updates separately, on its
own host.
