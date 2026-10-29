import { expect, it, vi } from "vitest";
import type { ComposerPaste } from "./composer-pastes";
import type { SecretFiles } from "./secret-paste";
import { SecretPreview } from "./secret-preview";

function fixture() {
  const paste: ComposerPaste = {
    token: "[Secret 12345678]",
    body: "(secret placed at /cxz/secrets/fixture/value)",
    bytes: 0,
    lines: 0,
    secret: {
      session: "s",
      run: "r",
      path: "/cxz/secrets/fixture/value",
      state: "ready",
    },
  };
  const bytes = new TextEncoder().encode(" private value\nsecond line ");
  const files: SecretFiles = {
    session: "s",
    run: "r",
    put: async () => "",
    read: vi.fn(async () => bytes),
    remove: async () => {},
  };
  return { paste, bytes, files };
}

it("reads a restored reference transiently, preserves the value, and clears it on close", async () => {
  const { paste, files, bytes } = fixture();
  const preview = new SecretPreview(paste, files);
  const close = preview.connect();
  await vi.waitFor(() => expect(preview.getSnapshot().state).toBe("ready"));
  expect(preview.getSnapshot().value).toBe(" private value\nsecond line ");
  expect(bytes.every((byte) => byte === 0)).toBe(true);
  expect(JSON.stringify(paste)).not.toContain("private value");
  close();
  expect(preview.getSnapshot().value).toBe("");
  expect(paste.secret?.listeners?.size).toBe(0);
});

it("preserves a leading BOM rather than changing the expanded value", async () => {
  const { paste, files } = fixture();
  files.read = async () => new TextEncoder().encode("\uFEFF private value\n");
  const preview = new SecretPreview(paste, files);
  const close = preview.connect();
  await vi.waitFor(() => expect(preview.getSnapshot().state).toBe("ready"));
  expect(preview.getSnapshot().value).toBe("\uFEFF private value\n");
  close();
});

it("ignores a late read after close and erases its returned bytes", async () => {
  const { paste, files, bytes } = fixture();
  let finish!: (bytes: Uint8Array) => void;
  let signal!: AbortSignal;
  files.read = vi.fn((_path: string, readSignal: AbortSignal) => {
    signal = readSignal;
    return new Promise<Uint8Array>((resolve) => {
      finish = resolve;
    });
  });
  const preview = new SecretPreview(paste, files);
  const close = preview.connect();
  close();
  expect(signal.aborted).toBe(true);
  finish(bytes);
  await vi.waitFor(() => expect(bytes.every((byte) => byte === 0)).toBe(true));
  expect(preview.getSnapshot().value).toBe("");
});

it("waits for preparation and does not read a different run or expose remote errors", async () => {
  const { paste, files } = fixture();
  paste.secret!.state = "uploading";
  paste.secret!.path = "";
  const preview = new SecretPreview(paste, files);
  const close = preview.connect();
  expect(files.read).not.toHaveBeenCalled();
  paste.secret!.state = "ready";
  paste.secret!.path = "/cxz/secrets/fixture/value";
  paste.secret!.listeners!.forEach((listener) => listener());
  await vi.waitFor(() => expect(preview.getSnapshot().state).toBe("ready"));
  close();
  const other = new SecretPreview(paste, { ...files, run: "other" });
  const stop = other.connect();
  expect(other.getSnapshot().state).toBe("error");
  expect(files.read).toHaveBeenCalledTimes(1);
  stop();
  const expired = new SecretPreview(paste, {
    ...files,
    read: async () => {
      throw new Error("server echoed private value");
    },
  });
  const dispose = expired.connect();
  await vi.waitFor(() => expect(expired.getSnapshot().state).toBe("error"));
  expect(expired.getSnapshot().error).not.toContain("private value");
  dispose();
});
