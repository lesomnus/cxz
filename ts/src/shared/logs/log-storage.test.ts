import { expect, it } from "vitest";
import { decodeLines, encodeLine, logEntry } from "./log-entry";
import { MemoryLogStorage, OPFSLogStorage } from "./log-storage";

const record = (message: string) =>
  logEntry(
    {
      level: "warn",
      category: "rpc",
      message,
      session: "session-uuid",
      connection: "https://cxz.example",
    },
    "test-tab",
    "revision",
  );

// A byte-addressable fake filesystem exercises the same append/rotate/reopen
// paths as OPFS. Real browser API/Worker/lock integration is covered separately.
class FileHandle {
  kind = "file";
  bytes = new Uint8Array();
  async getFile() {
    return new File([this.bytes], "log.jsonl");
  }
  async createWritable() {
    let position = 0;
    let next = this.bytes.slice();
    return {
      seek: async (value: number) => {
        position = value;
      },
      truncate: async (size: number) => {
        next = next.slice(0, size);
      },
      write: async (value: string) => {
        const bytes = new TextEncoder().encode(value);
        const grown = new Uint8Array(
          Math.max(next.length, position + bytes.length),
        );
        grown.set(next);
        grown.set(bytes, position);
        next = grown;
        position += bytes.length;
      },
      close: async () => {
        this.bytes = next;
      },
      abort: async () => {},
    };
  }
}
class Directory {
  kind = "directory";
  children = new Map<string, Directory | FileHandle>();
  async getDirectoryHandle(name: string) {
    if (!this.children.has(name)) this.children.set(name, new Directory());
    return this.children.get(name) as Directory;
  }
  async getFileHandle(name: string) {
    if (!this.children.has(name)) this.children.set(name, new FileHandle());
    return this.children.get(name) as FileHandle;
  }
  async *entries() {
    yield* this.children.entries();
  }
  async removeEntry(name: string) {
    this.children.delete(name);
  }
}
function fixture(segment = 1024, total = 3072) {
  const directory = new Directory();
  let queued: Promise<unknown> = Promise.resolve();
  const locks = {
    request: (_name: string, callback: () => Promise<unknown>) => {
      const next = queued.then(callback);
      queued = next.catch(() => {});
      return next;
    },
  } as unknown as LockManager;
  const open = () =>
    new OPFSLogStorage(
      directory as unknown as FileSystemDirectoryHandle,
      locks,
      segment,
      total,
    );
  return { directory, open };
}

it("redacts credential-like diagnostics and never retains arbitrary request objects", () => {
  const input = {
    level: "error",
    category: "rpc",
    message:
      'Bearer secret https://user:pass@cxz.example/test?token=abc#secret password=foo ghp_abcdef {"token":"json-secret"} api_key=key-secret',
    details: { token: "secret", stack: "sk-abcdefgh", count: 2 },
    requestBody: { prompt: "private conversation" },
  } as const;
  const result = logEntry(input, "tab", "version");
  const serialized = JSON.stringify(result);
  for (const secret of [
    "Bearer secret",
    "user:pass",
    "token=abc",
    "password=foo",
    "ghp_abcdef",
    "sk-abcdefgh",
    "json-secret",
    "key-secret",
    "private conversation",
  ])
    expect(serialized).not.toContain(secret);
  expect(result.details).toEqual({ stack: "[redacted]", count: 2 });
});
it("bounds memory in UTF-8 bytes and pages/searches/exports the same retained records", async () => {
  const store = new MemoryLogStorage(2000);
  await store.append(Array.from({ length: 30 }, (_, i) => record(`한글 ${i}`)));
  const page = await store.query({});
  expect(page.bytes).toBeLessThanOrEqual(2000);
  expect(page.total).toBeLessThan(30);
  expect(page.entries[0].message).toBe("한글 29");
  expect(
    decodeLines(
      await store.export({ search: "한글 29", session: "session-uuid" }),
    ),
  ).toHaveLength(1);
  expect((await store.query({ level: "error" })).total).toBe(0);
  await store.clear();
  expect((await store.query({})).bytes).toBe(0);
});
it("rotates globally by bytes and reopens retained JSONL after concurrent tab writes", async () => {
  const f = fixture();
  await Promise.all(
    Array.from({ length: 30 }, (_, i) =>
      f.open().append([record(`entry-${i}`)]),
    ),
  );
  const page = await f.open().query({});
  expect(page.storage).toBe("opfs");
  expect(page.bytes).toBeLessThanOrEqual(3072);
  expect(page.total).toBeGreaterThan(0);
  expect(page.total).toBeLessThan(30);
  expect(page.entries[0].message).toBe("entry-29");
  const folder = await f.directory.getDirectoryHandle("cxz-client-logs-v1");
  for (const file of folder.children.values())
    expect((file as FileHandle).bytes.length).toBeLessThanOrEqual(1024);
  expect(decodeLines(await f.open().export({})).map((e) => e.id)).toEqual(
    page.entries.map((e) => e.id).reverse(),
  );
  await f.open().clear();
  expect((await f.open().query({})).total).toBe(0);
});
it("ignores and repairs a torn final line before appending new records", async () => {
  const f = fixture();
  const store = f.open();
  const previous = record("before crash");
  await store.append([previous]);
  const folder = await f.directory.getDirectoryHandle("cxz-client-logs-v1");
  const file = [...folder.children.values()][0] as FileHandle;
  file.bytes = new TextEncoder().encode(
    encodeLine(previous) + '{"unfinished":',
  );
  expect((await store.query({})).total).toBe(1);
  await f.open().append([record("after reload")]);
  expect((await store.query({})).entries.map((e) => e.message)).toEqual([
    "after reload",
    "before crash",
  ]);
});
it("keeps UI pages bounded while exports include every matching record", async () => {
  const f = fixture(4096, 100_000);
  await f
    .open()
    .append(Array.from({ length: 130 }, (_, i) => record(`record ${i}`)));
  const page = await f.open().query({});
  expect(page.entries).toHaveLength(100);
  expect(page.total).toBe(130);
  expect((await f.open().query({ offset: 100 })).entries).toHaveLength(30);
  expect(decodeLines(await f.open().export({}))).toHaveLength(130);
});
