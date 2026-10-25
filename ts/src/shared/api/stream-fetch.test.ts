import { afterEach, expect, it, vi } from "vitest";
import { rpcFetch } from "./stream-fetch";

class Socket {
  static instances: Socket[] = [];
  binaryType = "";
  sent: unknown[] = [];
  closed = false;
  onopen?: () => void;
  onclose?: (event?: {
    code: number;
    reason: string;
    wasClean: boolean;
  }) => void;
  onerror?: () => void;
  onmessage?: (e: { data: unknown }) => void;
  constructor(public url: URL) {
    Socket.instances.push(this);
    queueMicrotask(() => this.onopen?.());
  }
  send(data: unknown) {
    this.sent.push(data);
  }
  close() {
    this.closed = true;
  }
  head(status = 200) {
    this.onmessage?.({
      data: JSON.stringify({
        status,
        headers: { "Content-Type": "application/connect+json" },
      }),
    });
  }
  data(bytes: number[]) {
    this.onmessage?.({ data: new Uint8Array(bytes).buffer });
  }
  done() {
    this.onmessage?.({ data: '{"done":true}' });
  }
}
afterEach(() => {
  vi.unstubAllGlobals();
  Socket.instances = [];
});
const init = (signal?: AbortSignal): RequestInit => ({
  method: "POST",
  headers: {
    "Content-Type": "application/connect+json",
    "Connect-Protocol-Version": "1",
  },
  body: new Uint8Array([0, 0, 0, 0, 2, 123, 125]),
  signal,
});
async function connected(signal?: AbortSignal) {
  vi.stubGlobal("WebSocket", Socket);
  const index = Socket.instances.length;
  const response = rpcFetch(
    "http://127.0.0.1:5173/cxz.SessionService/Events",
    init(signal),
  );
  await vi.waitFor(() => expect(Socket.instances[index]?.sent).toHaveLength(2));
  return { response, socket: Socket.instances[index] };
}
it("keeps unary calls and HTTPS streams on authenticated fetch", async () => {
  const fetch = vi.fn(
    async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response("ok"),
  );
  vi.stubGlobal("fetch", fetch);
  await rpcFetch("http://127.0.0.1:5173/cxz.SessionService/Send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  await rpcFetch("https://cxz.test/cxz.SessionService/Events", init());
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch.mock.calls[0][1]).toMatchObject({ credentials: "same-origin" });
});
it("preserves request bytes, response frames and end trailers without HTTP subscriptions", async () => {
  const { response, socket } = await connected();
  expect(String(socket.url)).toBe(
    "ws://127.0.0.1:5173/rpc-stream/cxz.SessionService/Events",
  );
  expect(
    JSON.parse(socket.sent[0] as string).headers["connect-protocol-version"],
  ).toBe("1");
  expect(new Uint8Array(socket.sent[1] as ArrayBuffer)).toEqual(
    new Uint8Array([0, 0, 0, 0, 2, 123, 125]),
  );
  socket.head();
  socket.data([0, 0, 0, 0, 1, 123]);
  socket.data([2, 0, 0, 0, 2, 123, 125]);
  socket.done();
  expect([...new Uint8Array(await (await response).arrayBuffer())]).toEqual([
    0, 0, 0, 0, 1, 123, 2, 0, 0, 0, 2, 123, 125,
  ]);
  expect(socket.closed).toBe(true);
});
it("preserves upstream HTTP errors for Connect to decode", async () => {
  const { response, socket } = await connected();
  socket.head(503);
  socket.data([...new TextEncoder().encode('{"code":"unavailable"}')]);
  socket.done();
  const result = await response;
  expect(result.status).toBe(503);
  expect(await result.json()).toEqual({ code: "unavailable" });
});
it("aborting a parked connection rejects the request and closes the socket", async () => {
  const controller = new AbortController();
  const { response, socket } = await connected(controller.signal);
  const assertion = expect(response).rejects.toThrow();
  controller.abort();
  await assertion;
  expect(socket.closed).toBe(true);
});
it("aborts streamed responses and does not treat an unexpected close as completion", async () => {
  const controller = new AbortController();
  const { response, socket } = await connected(controller.signal);
  socket.head();
  const reader = (await response).body!.getReader();
  const assertion = expect(reader.read()).rejects.toThrow();
  controller.abort();
  await assertion;
  expect(socket.closed).toBe(true);
  const next = await connected();
  next.socket.head();
  const body = (await next.response).body!.getReader();
  const closed = expect(body.read()).rejects.toThrow("disconnected");
  next.socket.onclose?.();
  await closed;
});
it("reader cancellation releases the subscription and queue growth is bounded", async () => {
  const first = await connected();
  first.socket.head();
  await (await first.response).body!.cancel();
  expect(first.socket.closed).toBe(true);
  const next = await connected();
  next.socket.head();
  await next.response;
  next.socket.onmessage?.({ data: new ArrayBuffer(8 << 20) });
  next.socket.data([1]);
  await expect((await next.response).arrayBuffer()).rejects.toThrow("too slow");
  expect(next.socket.closed).toBe(true);
});
it("preserves WebSocket close codes and reasons even after an error event", async () => {
  const { response, socket } = await connected();
  const failure = expect(response).rejects.toMatchObject({
    message: "RPC stream connection failed",
    closeCode: 1012,
    closeReason: "service restart",
    wasClean: true,
  });
  socket.onerror?.();
  socket.onclose?.({ code: 1012, reason: "service restart", wasClean: true });
  await failure;
  expect(socket.closed).toBe(true);
});
