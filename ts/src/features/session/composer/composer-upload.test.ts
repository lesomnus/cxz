import { afterEach, expect, test, vi } from "vitest";
import type { Connection } from "#src/shared/api/connection.ts";
import { uploadFile } from "./composer-upload";

class FakeRequest {
  static latest: FakeRequest;
  upload = { onprogress: null as ((event: ProgressEvent) => void) | null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  timeout = 0;
  status = 200;
  responseText = "";
  type = "application/json";
  method = "";
  url = "";
  body?: Blob;
  headers = new Map<string, string>();
  aborted = false;
  constructor() {
    FakeRequest.latest = this;
  }
  open(method: string, url: string | URL) {
    this.method = method;
    this.url = String(url);
  }
  send(body: Blob) {
    this.body = body;
  }
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
  setRequestHeader(name: string, value: string) {
    this.headers.set(name, value);
  }
  getResponseHeader() {
    return this.type;
  }
  progress(loaded: number, total: number) {
    this.upload.onprogress?.({
      loaded,
      total,
      lengthComputable: true,
    } as ProgressEvent);
  }
  reply(body: unknown, status = 200) {
    this.status = status;
    this.responseText = JSON.stringify(body);
    this.onload?.();
  }
}

const connection = {
  inProcess: false,
  baseUrl: "http://localhost:3000",
} as Connection;
afterEach(() => vi.unstubAllGlobals());

test("raw upload reports actual transfer bytes and waits for the path acknowledgement", async () => {
  vi.stubGlobal("XMLHttpRequest", FakeRequest);
  const file = new File(["x".repeat(100)], "한글.txt");
  const progress = vi.fn();
  const uploading = uploadFile(
    connection,
    "session",
    "run",
    file,
    new AbortController().signal,
    progress,
  );
  const request = FakeRequest.latest;
  expect(request.method).toBe("POST");
  const url = new URL(request.url);
  expect(url.pathname).toBe("/attachments/session");
  expect(Object.fromEntries(url.searchParams)).toEqual({
    run: "run",
    name: "한글.txt",
    size: "100",
  });
  expect(request.headers.get("Content-Type")).toBe("application/octet-stream");
  expect(request.body).toBe(file);
  request.progress(12, 100);
  request.progress(100, 100);
  expect(progress.mock.calls).toEqual([
    [12, 100],
    [100, 100],
  ]);
  const resolved = vi.fn();
  void uploading.then(resolved);
  await Promise.resolve();
  expect(resolved).not.toHaveBeenCalled();
  request.reply({ path: "/cxz/assets/session/file.txt" });
  await expect(uploading).resolves.toBe("/cxz/assets/session/file.txt");
  expect(request.upload.onprogress).toBeNull();
});

test("cancellation aborts the raw request even after all bytes have transferred", async () => {
  vi.stubGlobal("XMLHttpRequest", FakeRequest);
  const controller = new AbortController();
  const uploading = uploadFile(
    connection,
    "session",
    "run",
    new File(["body"], "file.txt"),
    controller.signal,
  );
  const request = FakeRequest.latest;
  request.progress(4, 4);
  controller.abort(new Error("cancelled"));
  await expect(uploading).rejects.toThrow("cancelled");
  expect(request.aborted).toBe(true);
  expect(request.onload).toBeNull();
  const previous = FakeRequest.latest;
  await expect(
    uploadFile(
      connection,
      "session",
      "run",
      new File([], "file.txt"),
      controller.signal,
    ),
  ).rejects.toThrow("cancelled");
  expect(FakeRequest.latest).toBe(previous);
});

test("server errors and invalid acknowledgements never mark an upload ready", async () => {
  vi.stubGlobal("XMLHttpRequest", FakeRequest);
  for (const [body, status, message] of [
    [{ message: "Workspace unavailable" }, 503, "Workspace unavailable"],
    [{ path: "relative.txt" }, 200, "without a file path"],
    [{}, 200, "without a file path"],
    [null, 200, "without a file path"],
  ] as const) {
    const uploading = uploadFile(
      connection,
      "session",
      "run",
      new File([], "empty.txt"),
      new AbortController().signal,
    );
    FakeRequest.latest.reply(body, status);
    await expect(uploading).rejects.toThrow(message);
  }
});

test("WASM uploads report consumed chunks while retaining bounded reads", async () => {
  const file = new File([new Uint8Array(300_000)], "data.bin");
  const progress = vi.fn();
  const sizes: number[] = [];
  const local = {
    inProcess: true,
    sessions: {
      upload: async (messages: AsyncIterable<{ content?: Uint8Array }>) => {
        for await (const message of messages) {
          if (message.content) sizes.push(message.content.length);
        }
        return { path: "/cxz/assets/wasm/data.bin" };
      },
    },
  } as unknown as Connection;
  await expect(
    uploadFile(
      local,
      "session",
      "run",
      file,
      new AbortController().signal,
      progress,
    ),
  ).resolves.toBe("/cxz/assets/wasm/data.bin");
  expect(sizes.reduce((sum, size) => sum + size, 0)).toBe(file.size);
  expect(Math.max(...sizes)).toBeLessThanOrEqual(256 * 1024);
  expect(progress.mock.calls).toEqual([
    [sizes[0], file.size],
    [file.size, file.size],
  ]);
});
