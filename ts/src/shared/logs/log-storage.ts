import {
  decodeLines,
  encodeLine,
  matchesLog,
  pageSize,
  retainedBytes,
  segmentBytes,
  type LogEntry,
  type LogPage,
  type LogQuery,
} from "./log-entry";

export interface LogStorage {
  append(entries: LogEntry[]): Promise<void>;
  query(query: LogQuery): Promise<LogPage>;
  export(query: LogQuery): Promise<string>;
  clear(): Promise<void>;
}

export class MemoryLogStorage implements LogStorage {
  private entries: { entry: LogEntry; bytes: number }[] = [];
  private bytes = 0;
  constructor(private limit = 2 * 1024 * 1024) {}
  async append(entries: LogEntry[]) {
    for (const entry of entries) {
      const bytes = new TextEncoder().encode(encodeLine(entry)).length;
      if (bytes > this.limit) continue;
      this.entries.push({ entry, bytes });
      this.bytes += bytes;
      while (this.bytes > this.limit) this.bytes -= this.entries.shift()!.bytes;
    }
  }
  async query(query: LogQuery): Promise<LogPage> {
    const entries = this.entries
      .map((e) => e.entry)
      .reverse()
      .filter((e) => matchesLog(e, query));
    const offset = Math.max(0, query.offset ?? 0);
    return {
      entries: entries.slice(offset, offset + pageSize),
      total: entries.length,
      bytes: this.bytes,
      capacity: this.limit,
      storage: "memory",
    };
  }
  async export(query: LogQuery) {
    return this.entries
      .filter(({ entry }) => matchesLog(entry, query))
      .map(({ entry }) => encodeLine(entry))
      .join("");
  }
  async clear() {
    this.entries = [];
    this.bytes = 0;
  }
}

// Each operation runs inside one origin-wide Web Lock, including pruning and
// clear. A tab never holds an open file between operations or writes a manifest
// another tab could overwrite. File names encode append order.
export class OPFSLogStorage implements LogStorage {
  constructor(
    private root: FileSystemDirectoryHandle,
    private locks: Pick<LockManager, "request">,
    private segmentLimit = segmentBytes,
    private totalLimit = retainedBytes,
  ) {}
  private async locked<T>(
    fn: (directory: FileSystemDirectoryHandle) => Promise<T>,
  ): Promise<T> {
    return await this.locks.request("cxz.client-logs.v1", async () =>
      fn(
        await this.root.getDirectoryHandle("cxz-client-logs-v1", {
          create: true,
        }),
      ),
    );
  }
  private async files(directory: FileSystemDirectoryHandle) {
    const files: { name: string; handle: FileSystemFileHandle; file: File }[] =
      [];
    const iterable = directory as FileSystemDirectoryHandle & {
      entries(): AsyncIterableIterator<[string, FileSystemHandle]>;
    };
    for await (const [name, handle] of iterable.entries()) {
      if (handle.kind === "file" && /^\d{16}-[\da-f-]+\.jsonl$/.test(name)) {
        const h = handle as FileSystemFileHandle;
        files.push({ name, handle: h, file: await h.getFile() });
      }
    }
    return files.sort((a, b) => a.name.localeCompare(b.name));
  }
  async append(entries: LogEntry[]) {
    if (!entries.length) return;
    return this.locked(async (directory) => {
      let files = await this.files(directory);
      let current = files.at(-1);
      if (
        current?.file.size &&
        (await current.file.slice(-1).text()) !== "\n"
      ) {
        const bytes = new Uint8Array(await current.file.arrayBuffer());
        const size = bytes.lastIndexOf(10) + 1;
        const repair = await current.handle.createWritable({
          keepExistingData: true,
        });
        await repair.truncate(size);
        await repair.close();
        current.file = await current.handle.getFile();
      }
      let lines: string[] = [];
      let bytes = current?.file.size ?? 0;
      let lastStamp = Number(current?.name.slice(0, 16) ?? 0);
      const flush = async () => {
        if (!current || !lines.length) return;
        const writable = await current.handle.createWritable({
          keepExistingData: true,
        });
        try {
          await writable.seek(current.file.size);
          await writable.write(lines.join(""));
          await writable.close();
        } catch (error) {
          await writable.abort().catch(() => {});
          throw error;
        }
        current.file = await current.handle.getFile();
        lines = [];
      };
      for (const entry of entries) {
        const line = encodeLine(entry);
        const size = new TextEncoder().encode(line).length;
        if (size > this.segmentLimit || size > this.totalLimit) continue;
        if (!current || bytes + size > this.segmentLimit) {
          await flush();
          lastStamp = Math.max(Date.now(), lastStamp + 1);
          const name =
            String(lastStamp).padStart(16, "0") +
            "-" +
            crypto.randomUUID() +
            ".jsonl";
          const handle = await directory.getFileHandle(name, { create: true });
          current = { name, handle, file: await handle.getFile() };
          files.push(current);
          bytes = 0;
        }
        lines.push(line);
        bytes += size;
      }
      await flush();
      // Read sizes again: another tab's segments and our partial last segment
      // all count toward the same limit, even after a reload or an unclean exit.
      files = await this.files(directory);
      let total = files.reduce((sum, file) => sum + file.file.size, 0);
      for (const file of files) {
        if (total <= this.totalLimit) break;
        await directory.removeEntry(file.name);
        total -= file.file.size;
      }
    });
  }
  async query(query: LogQuery): Promise<LogPage> {
    return this.locked(async (directory) => {
      const files = await this.files(directory);
      const result: LogEntry[] = [];
      const offset = Math.max(0, query.offset ?? 0);
      let total = 0;
      for (const { file } of files.reverse()) {
        for (const entry of decodeLines(await file.text()).reverse()) {
          if (!matchesLog(entry, query)) continue;
          if (total >= offset && result.length < pageSize) result.push(entry);
          total++;
        }
      }
      return {
        entries: result,
        total,
        bytes: files.reduce((sum, f) => sum + f.file.size, 0),
        capacity: this.totalLimit,
        storage: "opfs",
      };
    });
  }
  async export(query: LogQuery) {
    return this.locked(async (directory) => {
      const parts: string[] = [];
      for (const { file } of await this.files(directory)) {
        parts.push(
          ...decodeLines(await file.text())
            .filter((e) => matchesLog(e, query))
            .map(encodeLine),
        );
      }
      return parts.join("");
    });
  }
  async clear() {
    return this.locked(async (directory) => {
      for (const file of await this.files(directory))
        await directory.removeEntry(file.name);
    });
  }
}
