// The dev server as a hub: several cxz installations, each a connection, served
// from one origin under /c/@alias/ or /c/UUID/. It is the development stand-in for a
// gateway that holds connections (docs/plans/web-connections.md, "허브"):
// the browser talks to this origin only, and each installation's session --
// its gateway's HttpOnly cookie -- is kept here, server-side, never in the
// browser. Two gateways' cookies could not live in one browser origin anyway:
// both are named __Host-cxz, and a __Host- cookie must have Path=/.
//
// Unprefixed paths stay what they were: the first connection, with its cookie
// in the browser as before, so a UI that knows nothing of connections keeps
// working.

import { createHash } from "node:crypto";

export const connectionPrefix = "/c/";
export const connectionName = /^[a-z0-9][a-z0-9-]{0,39}$/;
export const connectionId =
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
export const listPath = "/cxz.ConnectionService/List";

export function connectionIDs(entries) {
  const ids = new Set();
  const names = new Set();
  return entries.map((entry) => {
    if (
      typeof entry.name !== "string" ||
      !connectionName.test(entry.name) ||
      names.has(entry.name)
    ) {
      throw new Error(`Invalid or duplicate connection alias: ${entry.name}`);
    }
    names.add(entry.name);
    let id = entry.id;
    if (id === undefined) {
      const bytes = createHash("sha256")
        .update("cxz.web.connection\0")
        .update(entry.configFile ?? entry.origin)
        .digest();
      bytes[6] = (bytes[6] & 0x0f) | 0x80;
      bytes[8] = (bytes[8] & 0x3f) | 0x80;
      id = bytes
        .subarray(0, 16)
        .toString("hex")
        .replace(/^(\w{8})(\w{4})(\w{4})(\w{4})(\w{12})$/, "$1-$2-$3-$4-$5");
    }
    if (typeof id !== "string" || !connectionId.test(id)) {
      throw new Error(`Invalid connection UUID for ${entry.name}`);
    }
    id = id.toLowerCase();
    if (ids.has(id))
      throw new Error(`Duplicate connection UUID for ${entry.name}`);
    ids.add(id);
    return { ...entry, id };
  });
}

// connectionOf splits a scoped URL into the connection and its gateway path.
// A path outside /c/ is the default connection's, unchanged.
export function connectionOf(url, connections) {
  if (!url.startsWith(connectionPrefix)) {
    return { connection: connections[0], path: url, hub: false };
  }
  const rest = url.slice(connectionPrefix.length);
  const slash = rest.indexOf("/");
  const handle = (slash < 0 ? rest : rest.slice(0, slash)).replace(
    /^%40/i,
    "@",
  );
  const connection = handle.startsWith("@")
    ? connections.find((c) => c.name === handle.slice(1))
    : connectionId.test(handle) &&
      connections.find((c) => c.id === handle.toLowerCase());
  if (!connection) return undefined;
  return { connection, path: slash < 0 ? "/" : rest.slice(slash), hub: true };
}

// Sessions are the hub's half of each connection: the cookies its gateway set,
// kept per connection and sent only to that gateway.
export class Sessions {
  #cookies = new Map();
  cookie(name) {
    const jar = this.#cookies.get(name);
    if (!jar) return "";
    return [...jar].map(([k, v]) => `${k}=${v}`).join("; ");
  }
  // take keeps what a gateway's Set-Cookie said, and removes what it cleared.
  take(name, setCookies) {
    const jar = this.#cookies.get(name) ?? new Map();
    for (const header of setCookies ?? []) {
      const [pair, ...attributes] = header.split(";");
      const eq = pair.indexOf("=");
      if (eq < 1) continue;
      const key = pair.slice(0, eq).trim();
      const value = pair.slice(eq + 1).trim();
      const expired = attributes.some((a) => {
        const [k, v] = a.split("=").map((s) => s.trim().toLowerCase());
        return (
          (k === "max-age" && Number(v) <= 0) ||
          (k === "expires" && Date.parse(v) < Date.now())
        );
      });
      if (!value || expired) jar.delete(key);
      else jar.set(key, value);
    }
    if (jar.size) this.#cookies.set(name, jar);
    else this.#cookies.delete(name);
  }
  forget(name) {
    this.#cookies.delete(name);
  }
}

// The answer to ConnectionService.List, in Connect's JSON: what this hub is
// configured with. Every connection is read-only here -- the hub's
// configuration says which there are, not the browser -- and the first is the
// default, the one unprefixed paths reach. A connection's route is
// /c/@alias or /c/UUID; target is the installation it reaches, for showing.
export function listConnections(connections) {
  return {
    items: connections.map((c, i) => ({
      id: Buffer.from(c.id.replaceAll("-", ""), "hex").toString("base64"),
      name: c.name,
      target: c.origin,
      transport: "gateway",
      isDefault: i === 0,
      readOnly: true,
    })),
  };
}

export function answerList(req, res, connections) {
  res.setHeader("Cache-Control", "no-store");
  const type = (req.headers["content-type"] ?? "").split(";")[0].trim();
  if (req.method !== "POST" || type !== "application/json") {
    // The UI speaks Connect JSON; a hub with no protobuf schemas at hand
    // answers only that, and says so in Connect's own error shape.
    res.writeHead(415, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        code: "unimplemented",
        message:
          "the dev hub answers ConnectionService.List in Connect JSON only",
      }),
    );
    return;
  }
  // The request body is drained, not read: List takes no filter here.
  req.resume();
  req.on("end", () => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(listConnections(connections)));
  });
}
