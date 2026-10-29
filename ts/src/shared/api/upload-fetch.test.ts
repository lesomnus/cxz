import { afterEach, expect, it, vi } from "vitest";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { uploadFetch } from "./upload-fetch";

class XHR {
  static instances: XHR[] = [];
  upload: {
    onprogress?:
      | null
      | ((event: {
          lengthComputable: boolean;
          loaded: number;
          total: number;
        }) => void);
  } = {};
  onload?: null | (() => void);
  onerror?: null | (() => void);
  onabort?: null | (() => void);
  ontimeout?: null | (() => void);
  status = 200;
  statusText = "OK";
  response = new TextEncoder().encode('{"path":"/cxz/secrets/fixture/value"}')
    .buffer;
  headers = new Headers();
  body?: ArrayBuffer;
  aborted = false;
  constructor() {
    XHR.instances.push(this);
  }
  open(_method: string, _url: string) {}
  setRequestHeader(name: string, value: string) {
    this.headers.set(name, value);
  }
  send(body: ArrayBuffer) {
    this.body = body;
  }
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
  getAllResponseHeaders() {
    return "content-type: application/json\r\nconnect-protocol-version: 1\r\n";
  }
}
afterEach(() => {
  vi.unstubAllGlobals();
  XHR.instances = [];
});

it("preserves a real Connect unary exchange while forwarding measured upload progress", async () => {
  vi.stubGlobal("XMLHttpRequest", XHR);
  const progress = vi.fn();
  const client = createClient(
    ProjectService,
    createConnectTransport({
      baseUrl: "https://cxz.test",
      fetch: uploadFetch(progress),
    }),
  );
  const bytes = new TextEncoder().encode("private");
  const call = client.putSecretFile({
    ref: { key: { case: "alias", value: "project" } },
    session: "s/r",
    secret: bytes,
  });
  await vi.waitFor(() => expect(XHR.instances[0]?.body).toBeDefined());
  const xhr = XHR.instances[0];
  const wire = new TextDecoder().decode(xhr.body);
  expect(JSON.parse(wire).secret).toBe(btoa("private"));
  expect(xhr.headers.get("Connect-Protocol-Version")).toBe("1");
  xhr.upload.onprogress?.({ lengthComputable: true, loaded: 5, total: 10 });
  expect(progress).toHaveBeenLastCalledWith(5, 10);
  xhr.onload?.();
  expect((await call).path).toBe("/cxz/secrets/fixture/value");
  expect(new Uint8Array(xhr.body!).every((byte) => byte === 0)).toBe(true);
});

it("cancels an in-flight upload and clears the serialized request buffer", async () => {
  vi.stubGlobal("XMLHttpRequest", XHR);
  const abort = new AbortController();
  const pending = uploadFetch(() => {})("https://cxz.test/upload", {
    method: "POST",
    body: "private",
    signal: abort.signal,
  });
  const rejected = expect(pending).rejects.toThrow();
  await vi.waitFor(() => expect(XHR.instances[0]?.body).toBeDefined());
  const xhr = XHR.instances[0];
  abort.abort();
  await rejected;
  expect(xhr.aborted).toBe(true);
  expect(new Uint8Array(xhr.body!).every((byte) => byte === 0)).toBe(true);
});
