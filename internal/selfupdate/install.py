"""One-directory installation helper. Invoked only through a cxz Docker client."""
import fcntl
import hashlib
import json
import os
import select
import shutil
import signal
import stat
import sys
import tempfile


cancelled = False
input_buffer = b""
input_fd = sys.stdin.fileno()
os.set_blocking(input_fd, False)


def interrupted(_signal, _frame):
    # Do not inject an exception into an arbitrary filesystem/buffer operation:
    # cleanup state may not yet be recorded when the signal arrives.
    global cancelled
    cancelled = True


signal.signal(signal.SIGTERM, interrupted)


def check_cancelled():
    if cancelled:
        raise RuntimeError("installation cancelled")


def read_client(size):
    global input_buffer
    check_cancelled()
    while not input_buffer:
        ready, _, _ = select.select([input_fd], [], [], 0.1)
        check_cancelled()
        if not ready:
            continue
        try:
            input_buffer = os.read(input_fd, max(size, 4096))
        except BlockingIOError:
            continue
        if not input_buffer:
            return b""
    chunk, input_buffer = input_buffer[:size], input_buffer[size:]
    return chunk


def reply(stage, **values):
    print(json.dumps(dict(stage=stage, **values)), flush=True)


def message():
    line = bytearray()
    for _ in range(4096):
        value = read_client(1)
        if value == b"\n":
            return json.loads(line)
        if not value:
            break
        line.extend(value)
    raise RuntimeError("installation client disconnected or sent an invalid request")


def digest(stream):
    stream.seek(0)
    value = hashlib.file_digest(stream, "sha256").hexdigest()
    stream.seek(0)
    return value


def open_target(name):
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW)
    if not stat.S_ISREG(os.fstat(fd).st_mode):
        os.close(fd)
        raise RuntimeError("installation target is not a regular file")
    return os.fdopen(fd, "rb")


def identity(st):
    return (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns,
            st.st_uid, st.st_gid, st.st_mode)


def finish_file(stream, original):
    stream.flush()
    os.fchown(stream.fileno(), original.st_uid, original.st_gid)
    os.fchmod(stream.fileno(), original.st_mode & 0o777)
    os.fsync(stream.fileno())


def install():
    directory, name, marker, token, expected, uid, gid, mode = sys.argv[1:]
    for part in (name, marker):
        if part in ("", ".", "..") or os.path.basename(part) != part:
            raise RuntimeError("invalid installation filename")
    os.chdir(directory)
    marker_created = False
    temporary = []
    changed = False
    lock = None
    try:
        with open_target(name) as old:
            original = os.fstat(old.fileno())
            if digest(old) != expected or (original.st_uid, original.st_gid,
                    original.st_mode & 0o777) != (int(uid), int(gid), int(mode)):
                raise RuntimeError("Docker sees a different executable or ownership at this path")

        # A fresh challenge must be visible through the client's own filesystem.
        # Matching versions/checksums alone cannot distinguish two different hosts.
        fd = os.open(marker, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o644)
        marker_created = True
        with os.fdopen(fd, "w") as proof:
            proof.write(token)
            proof.flush()
            os.fchmod(proof.fileno(), 0o644)
            os.fsync(proof.fileno())
        reply("probe")
        if message() != {"continue": True}:
            raise RuntimeError("client could not verify the installation directory")
        os.unlink(marker)
        marker_created = False

        fd = os.open("." + name + ".update.lock", os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
        lock = os.fdopen(fd, "r+b")
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with open_target(name) as old:
            if identity(os.fstat(old.fileno())) != identity(original) or digest(old) != expected:
                raise RuntimeError("executable changed before the installer acquired its lock")
            reply("ready")
            header = message()
            size, checksum = header.get("size"), header.get("sha256")
            if not isinstance(size, int) or not 0 < size <= 512 * 1024 * 1024:
                raise RuntimeError("invalid executable size")
            if not isinstance(checksum, str) or len(checksum) != 64:
                raise RuntimeError("invalid executable checksum")
            fd, candidate = tempfile.mkstemp(prefix=".cxz-update-", dir=".")
            temporary.append(candidate)
            with os.fdopen(fd, "wb") as output:
                received = hashlib.sha256()
                remaining = size
                while remaining:
                    chunk = read_client(min(remaining, 1024 * 1024))
                    if not chunk:
                        raise RuntimeError("executable transfer interrupted")
                    output.write(chunk)
                    received.update(chunk)
                    remaining -= len(chunk)
                if received.hexdigest() != checksum:
                    raise RuntimeError("transferred executable checksum differs")
                finish_file(output, original)
            check_cancelled()

            if identity(os.stat(name, follow_symlinks=False)) != identity(original) or digest(old) != expected:
                raise RuntimeError("executable changed during the build")
            fd, backup = tempfile.mkstemp(prefix=".cxz-previous-", dir=".")
            temporary.append(backup)
            with os.fdopen(fd, "wb") as output:
                shutil.copyfileobj(old, output)
                finish_file(output, original)
            previous = name[:-4] + ".previous.exe" if name.lower().endswith(".exe") else name + ".previous"
            os.replace(backup, previous)
            dirfd = os.open(".", os.O_RDONLY | os.O_DIRECTORY)
            try:
                # Persist the backup before making the new executable visible.
                os.fsync(dirfd)
                check_cancelled()
                os.replace(candidate, name)
                changed = True
                os.fsync(dirfd)
            finally:
                os.close(dirfd)
            reply("done", changed=True)
    except Exception as error:
        reply("error", changed=changed, error=str(error))
    finally:
        if marker_created:
            os.unlink(marker)
        for path in temporary:
            try:
                os.unlink(path)
            except FileNotFoundError:
                pass
        if lock is not None:
            lock.close()


install()
