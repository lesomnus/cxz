import { WebSocketServer } from "ws";

const prefix = "/rpc-stream";
const rpcPath = /^\/(cxz|payday)\.[A-Za-z0-9_.]+\/[A-Za-z][A-Za-z0-9_]*$/;
const blocked =
  /^(host|cookie|authorization|origin|connection|upgrade|content-length|transfer-encoding|proxy-.*|sec-.*)$/i;

// Forward Connect envelopes without parsing protobuf or dispatching RPC methods.
// This terminates the local development socket, so an older installed gateway
// does not need a new endpoint just to debug its UI through HTTP/1.1 Vite.
//
// route answers where a request's stream goes: {origin, path, cookie}, with
// path the request's own once any connection prefix is off. A gateway object
// is the one-connection form: every stream goes to it with the browser's
// cookie.
export function devRPCStreams(server, route, localRequest) {
  if (typeof route !== "function") {
    const gateway = route;
    route = (req) => ({
      origin: gateway.origin,
      path: req.url || "",
      cookie: req.headers.cookie || "",
    });
  }
  const sockets = new WebSocketServer({ noServer: true, maxPayload: 8 << 20 });
  const upgrade = (req, socket, head) => {
    const target = route(req);
    if (!target?.path.startsWith(prefix + "/")) return;
    if (!localRequest(req) || !rpcPath.test(target.path.slice(prefix.length))) {
      socket.destroy();
      return;
    }
    sockets.handleUpgrade(req, socket, head, (ws) => bridge(ws, target));
  };
  server.httpServer.prependListener("upgrade", upgrade);
  server.httpServer.once("close", () => {
    server.httpServer.removeListener("upgrade", upgrade);
    for (const socket of sockets.clients) socket.terminate();
    sockets.close();
  });
}

function bridge(socket, target) {
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
      target.origin + target.path.slice(prefix.length),
      {
        method: "POST",
        headers: {
          ...headers,
          Origin: target.origin,
          Cookie: target.cookie,
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
