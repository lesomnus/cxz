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

// A connection whose server is in the page gets the same request every other
// connection gets. What differs is only who carries it, so this pins the
// request rather than the carrying: a transfer that saw a different URL, or a
// caller that trusted a transfer's answer without checking it, is the failure.
test("a connection that carries its own bodies still makes the same request", async () => {
  vi.stubGlobal("XMLHttpRequest", FakeRequest);
  const previous = FakeRequest.latest;
  const file = new File([new Uint8Array(300_000)], "한글.bin");
  const progress = vi.fn();
  const seen: { url: string; size: number }[] = [];
  const local = {
    inProcess: true,
    baseUrl: "https://sandbox.invalid",
    transfer: async (url: URL, carried: File) => {
      seen.push({ url: url.toString(), size: carried.size });
      return "/cxz/assets/wasm/한글.bin";
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
  ).resolves.toBe("/cxz/assets/wasm/한글.bin");
  expect(seen).toEqual([
    {
      url: "https://sandbox.invalid/attachments/session?run=run&name=%ED%95%9C%EA%B8%80.bin&size=300000",
      size: file.size,
    },
  ]);
  // The request never went out, and the acknowledgement is still checked.
  expect(FakeRequest.latest).toBe(previous);
  await expect(
    uploadFile(
      {
        ...local,
        transfer: async () => "relative.bin",
      } as unknown as Connection,
      "session",
      "run",
      file,
      new AbortController().signal,
    ),
  ).rejects.toThrow("without a file path");
});
