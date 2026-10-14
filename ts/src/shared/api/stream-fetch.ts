// HTTP/1.1 has a small origin-wide connection pool, shared by every tab.
// Connect's long-lived subscriptions use WebSockets on plaintext origins so
// ordinary reads and writes never wait behind those subscriptions. HTTPS keeps
// the normal Connect fetch transport (HTTP/2 multiplexes its requests).
export const rpcFetch: typeof fetch = (input, init) => {
  const url = new URL(input instanceof Request ? input.url : String(input));
  const headers = new Headers(init?.headers);
  if (
    url.protocol !== "http:" ||
    !/^application\/connect\+(json|proto)$/.test(
      headers.get("Content-Type") ?? "",
    )
  )
    return fetch(input, { ...init, credentials: "same-origin" });
  return streamFetch(url, init ?? {}, headers);
};

async function streamFetch(url: URL, init: RequestInit, headers: Headers) {
  const body = await new Response(init.body).arrayBuffer();
  init.signal?.throwIfAborted();
  url.protocol = "ws:";
  url.pathname = "/rpc-stream" + url.pathname;
  const socket = new WebSocket(url);
  socket.binaryType = "arraybuffer";
  return new Promise<Response>((resolve, reject) => {
    let controller: ReadableStreamDefaultController<Uint8Array>;
    let receivedHead = false,
      ended = false;
    const cleanup = () => {
      init.signal?.removeEventListener("abort", aborted);
      socket.close();
    };
    const fail = (error: unknown) => {
      if (ended) return;
      ended = true;
      reject(error);
      controller.error(error);
      cleanup();
    };
    const aborted = () =>
      fail(init.signal?.reason ?? new DOMException("Aborted", "AbortError"));
    const stream = new ReadableStream<Uint8Array>(
      {
        start(value) {
          controller = value;
        },
        cancel() {
          ended = true;
          cleanup();
        },
      },
      new ByteLengthQueuingStrategy({ highWaterMark: 8 << 20 }),
    );
    socket.onopen = () => {
      socket.send(
        JSON.stringify({ headers: Object.fromEntries(headers.entries()) }),
      );
      socket.send(body);
    };
    socket.onmessage = (event) => {
      if (ended) return;
      try {
        if (typeof event.data === "string") {
          const frame = JSON.parse(event.data);
          if (frame.error) throw new TypeError(frame.error);
          if (frame.done && receivedHead) {
            ended = true;
            controller.close();
            cleanup();
          } else if (
            !receivedHead &&
            Number.isInteger(frame.status) &&
            frame.status >= 200 &&
            frame.status <= 599
          ) {
            receivedHead = true;
            resolve(
              new Response(stream, {
                status: frame.status,
                headers: frame.headers,
              }),
            );
          } else throw new TypeError("Invalid RPC stream response");
        } else {
          if (!receivedHead) throw new TypeError("Missing RPC stream headers");
          if (event.data.byteLength > (controller.desiredSize ?? 0))
            throw new TypeError("RPC stream consumer is too slow");
          controller.enqueue(new Uint8Array(event.data));
        }
      } catch (error) {
        fail(error);
      }
    };
    socket.onerror = () => fail(new TypeError("RPC stream connection failed"));
    socket.onclose = () => fail(new TypeError("RPC stream disconnected"));
    init.signal?.addEventListener("abort", aborted, { once: true });
    if (init.signal?.aborted) aborted();
  });
}
