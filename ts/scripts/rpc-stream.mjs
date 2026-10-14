import { WebSocketServer } from "ws";

const prefix = "/rpc-stream";
const rpcPath = /^\/(cxz|payday)\.[A-Za-z0-9_.]+\/[A-Za-z][A-Za-z0-9_]*$/;
const blocked =
  /^(host|cookie|authorization|origin|connection|upgrade|content-length|transfer-encoding|proxy-.*|sec-.*)$/i;

// Forward Connect envelopes without parsing protobuf or dispatching RPC methods.
// This terminates the local development socket, so an older installed gateway
// does not need a new endpoint just to debug its UI through HTTP/1.1 Vite.
export function devRPCStreams(server, gateway, localRequest) {
  const sockets = new WebSocketServer({ noServer: true, maxPayload: 8 << 20 });
  const upgrade = (req, socket, head) => {
    if (!(req.url || "").startsWith(prefix + "/")) return;
    if (!localRequest(req) || !rpcPath.test(req.url.slice(prefix.length))) {
      socket.destroy();
      return;
    }
    sockets.handleUpgrade(req, socket, head, (ws) => bridge(ws, req, gateway));
  };
  server.httpServer.prependListener("upgrade", upgrade);
  server.httpServer.once("close", () => {
    server.httpServer.removeListener("upgrade", upgrade);
    for (const socket of sockets.clients) socket.terminate();
    sockets.close();
  });
}

function bridge(socket, request, gateway) {
  const active = new AbortController();
  let headers,
    started = false;
  const timeout = setTimeout(() => socket.terminate(), 10000);
  const stop = () => {
    clearTimeout(timeout);
    active.abort();
  };
  socket.on("close", stop);
  socket.on("error", stop);
  socket.on("message", (data, binary) => {
    if (started) return socket.close(1008, "Unexpected input");
    if (!headers) {
      try {
        if (binary || data.length > 65536) throw new Error();
        const frame = JSON.parse(data.toString());
        headers = Object.fromEntries(
          Object.entries(frame.headers).filter(([name]) => !blocked.test(name)),
        );
        if (
          !/^application\/connect\+(json|proto)$/.test(
            headers["content-type"] ?? "",
          )
        )
          throw new Error();
      } catch {
        socket.close(1008, "Invalid request");
      }
      return;
    }
    if (!binary) return socket.close(1008, "Invalid request body");
    started = true;
    clearTimeout(timeout);
    void forward(data)
      .catch(() => {
        if (socket.readyState === socket.OPEN)
          socket.close(1011, "RPC upstream unavailable");
      })
      .finally(() => {
        stop();
        socket.close();
      });
  });
  const send = (data) =>
    new Promise((resolve, reject) =>
      socket.send(data, (error) => (error ? reject(error) : resolve())),
    );
  async function forward(body) {
    const response = await fetch(
      gateway.origin + request.url.slice(prefix.length),
      {
        method: "POST",
        headers: {
          ...headers,
          Origin: gateway.origin,
          Cookie: request.headers.cookie || "",
        },
        body,
        redirect: "error",
        signal: active.signal,
      },
    );
    await send(
      JSON.stringify({
        status: response.status,
        headers: Object.fromEntries(response.headers),
      }),
    );
    for await (const chunk of response.body) await send(chunk);
    await send(JSON.stringify({ done: true }));
  }
}
