# File attachments

Type an opening backtick followed immediately by `!` at the attachment position
inside your message, then drop, paste or type a host file path. For example:

```text
Compare `!/home/me/report.pdf` with the current implementation.
Compare `!"C:\Users\me\My Report.pdf"` with the current implementation.
```

The path becomes a chip after 300 ms without draft changes. Closing the backtick
requests an immediate check. This also handles terminals that deliver a drop one
character at a time. Invalid, incomplete or missing paths remain editable. Keep
surrounding prose outside the closing backtick, or continue after the chip appears.
Ctrl+S retains the draft while an explicit host reference is unresolved.

The `!` marker selects the filesystem of the machine running the **cxz client**,
even when connected to a remote daemon. Host path suggestions use that filesystem;
Tab browses, and Enter opens a directory or finishes a file. Backticks without `!`
continue to browse the session container. Bare dropped/pasted paths stay as text.
No drag/drop detection or typing-speed inference selects the filesystem.

Quoted paths, spaces, Unicode filenames, `file://` URLs, `~/` home paths, and
multiple space-separated paths inside one marker are supported. Only regular
files are accepted, at most 16 per marker and 1 GiB per file. The client must be
able to read the source: a local client can upload to a remote daemon, whereas
an SSH-hosted client cannot read a file on the user's PC by its local path.
Paths are parsed without executing shell substitutions.

Each file becomes `[filename ⣿⣿⣤⣀⣀]`. The five cells use the same Braille levels
as quota/usage bars and advance as the client reads bytes into the upload stream.
On completion, the bar becomes a right-aligned size of at most five characters (`1.23K`,
`12.3K`, `123K`, `1.23M`; K/M/G use powers of 1024). Small files use B. Failed
uploads show `ERROR`. Long display names are truncated; the stored original name
is preserved. Duplicate display names receive a numbered suffix.

Left/Right selects a chip; Enter shows details, `f` retries a failed upload, and
`d` or Backspace/Delete removes it. Removal cancels an in-flight upload. Ctrl+S
keeps the draft while any attached file is uploading or failed. Completed chips
expand to an agent-readable path. Removing a completed chip does not delete its
asset, so previously sent references remain valid. These chips are independent
of existing large-text paste chips and their text/file toggle.

## Storage and protocol

The manager owns `<state>/assets`. This resides in its persistent state volume
in managed installations; project containers do not receive the manager volume.

- `cas/`: one flob `OsStores` pool, using the runtime session ID as the namespace.
  Identical content across sessions shares an inode.
- `exports/<project>/<session>/<asset>/<filename>`: hard links to committed
  blobs, readable as mode 0444. Only this project's export directory is mounted
  at `/cxz/assets`, read-only. The CAS stays outside that mount. Filenames are
  preserved by these links, without filename labels or JSON sidecars. Separate attachment directories
  distinguish identical filenames, including when their contents differ.

The daemon translates its export path through its inspected Docker mount source
before provisioning the project. It returns `/cxz/assets/<session>/<asset>/<name>`
to the client; a standalone runtime returns its local export path. This avoids
confusing client, manager-container, engine-host and agent-container paths.
The OS backend's `Open` currently returns `*os.File`; that pinned implementation
is used in one adapter to obtain the source path for hard links. No private flob
sharding layout is reconstructed.

`Upload` is one client-streaming RPC: a filename/size/session header followed by
file bytes. The daemon feeds that stream to `flob.Add` and publishes the hard
link only after a complete, size-checked upload. There is no cxz upload ID,
offset protocol, staging record or resume operation; retry starts a new stream.
The header exists only for that call. flob manages its own temporary file and
removes it when Add returns, including on read failure or cancellation.
Completed hard links remain with daemon state.
Uploaded files are snapshots; changes to the original file require reattachment.

New or recreated projects receive the mount. Existing containers require
`cxz project recreate WORKSPACE` before file uploads; cxz reports this without
recreating them automatically. Project recreation replaces its writable layer.
