import { afterEach, describe, expect, it } from "vitest";
import { createServer as createHttpServer, request } from "node:http";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";
import WebSocket from "ws";
import {
  devGatewayPlugin,
  loadDevConnections,
  loadDevGateway,
} from "./web-dev.mjs";

const token = "test-only-token-".repeat(4);
const cleanup = [];
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});

async function temp() {
  const dir = await mkdtemp(join(tmpdir(), "cxz-web-dev-"));
  cleanup.push(() => rm(dir, { recursive: true, force: true }));
  return dir;
}

async function config(dir, name = "web.json", values = {}) {
  await mkdir(dir, { recursive: true });
  await writeFile(join(dir, "web-token"), token + "\n");
  await writeFile(
    join(dir, name),
    JSON.stringify({
      origin: "http://127.0.0.1:7350",
      access_token_file: "web-token",
      ...values,
    }),
  );
}

describe("dev gateway discovery", () => {
  it("uses the same default and XDG state locations as cxz", async () => {
    const home = await temp();
    const state = join(home, ".local", "state", "cxz");
    await config(state);
    expect((await loadDevGateway({}, home)).tokenFile).toBe(
      join(state, "web-token"),
    );
    const xdg = join(home, "custom-state");
    await config(join(xdg, "cxz"));
    expect(
      (await loadDevGateway({ XDG_STATE_HOME: xdg }, home)).configFile,
    ).toBe(join(xdg, "cxz", "web.json"));
  });

  it("prefers installed flag overrides and respects CXZ_STATE", async () => {
    const state = await temp();
    await config(state);
    await writeFile(join(state, "custom-token"), token);
    await config(state, "web-installation.json", {
      origin: "http://127.0.0.1:7351",
      access_token_file: "custom-token",
    });
    expect(
      await loadDevGateway({ CXZ_STATE: state, XDG_STATE_HOME: "/unused" }),
    ).toMatchObject({
      origin: "http://127.0.0.1:7351",
      tokenFile: join(state, "custom-token"),
    });
  });

  it("resolves token paths relative to an explicitly selected configuration", async () => {
    const dir = await temp();
    const tokenFile = join(dir, "web-token");
    await writeFile(tokenFile, token);
    await config(join(dir, "config"), "custom.json", {
      access_token_file: "../web-token",
    });
    expect(
      (
        await loadDevGateway({
          CXZ_WEB_CONFIG: join(dir, "config", "custom.json"),
        })
      ).tokenFile,
    ).toBe(tokenFile);
  });

  it("reports absent configuration without generating files", async () => {
    const dir = await temp();
    await expect(loadDevGateway({ CXZ_STATE: dir })).rejects.toThrow(
      "Run cxz web up",
    );
  });

  it("refuses an invalid installed configuration instead of silently falling back", async () => {
    const dir = await temp();
    await config(dir);
    await writeFile(join(dir, "web-installation.json"), "{");
    await expect(loadDevGateway({ CXZ_STATE: dir })).rejects.toThrow(
      "Invalid cxz web configuration",
    );
  });

  it("rejects a malformed token without including its contents in errors", async () => {
    const dir = await temp();
    await config(dir);
    await writeFile(join(dir, "web-token"), token + " secret");
    await expect(loadDevGateway({ CXZ_STATE: dir })).rejects.toThrow(
      `Invalid cxz web token file: ${join(dir, "web-token")}`,
    );
  });
});

async function fixture({
  loginStatus = 204,
  publicToken = false,
  streamRPC = false,
  identity,
} = {}) {
  const dir = await temp();
  const tokenFile = join(
    dir,
    ...(publicToken ? ["public", "web-token"] : ["web-token"]),
  );
  if (publicToken) await mkdir(join(dir, "public"));
  await writeFile(tokenFile, token);
  const calls = [];
  const gateway = createHttpServer(async (req, res) => {
    const parts = [];
    for await (const part of req) parts.push(part);
    calls.push({
      path: req.url,
      headers: req.headers,
      body: Buffer.concat(parts).toString(),
    });
    if (req.url === "/auth/status") {
      if (identity && req.headers.cookie === "test-session=valid")
        res.setHeader("Payday-Identity", identity);
      res.writeHead(req.headers.cookie === "test-session=valid" ? 204 : 401);
      res.end();
    } else if (req.url === "/auth/login") {
      if (identity && loginStatus === 204)
        res.setHeader("Payday-Identity", identity);
      res.writeHead(
        loginStatus,
        loginStatus === 204
          ? {
              "Set-Cookie":
                "test-session=valid; Path=/; Secure; HttpOnly; SameSite=Strict",
            }
          : {},
      );
      res.end(loginStatus === 204 ? undefined : token);
    } else if (streamRPC && req.url === "/cxz.SessionService/Events") {
      res.writeHead(200, { "Content-Type": "application/connect+json" });
      res.write(Buffer.from([0, 0, 0, 0, 2, 123, 125]));
      res.on("close", () => calls.push({ path: "stream canceled" }));
    } else {
      res.end("proxied");
    }
  });
  await new Promise((resolve) => gateway.listen(0, "127.0.0.1", resolve));
  cleanup.push(
    () =>
      new Promise((resolve) => {
        gateway.close(resolve);
        gateway.closeAllConnections();
      }),
  );
  const origin = `http://127.0.0.1:${gateway.address().port}`;
  const portReservation = createHttpServer();
  await new Promise((resolve) =>
    portReservation.listen(0, "127.0.0.1", resolve),
  );
  const port = portReservation.address().port;
  await new Promise((resolve) => portReservation.close(resolve));
  const vite = await createServer({
    root: dir,
    configFile: false,
    plugins: [devGatewayPlugin({ origin, tokenFile })],
    logLevel: "silent",
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { host: "127.0.0.1", port, strictPort: true, watch: null },
  });
  await vite.listen();
  cleanup.push(() => vite.close());
  return {
    url: `http://127.0.0.1:${vite.httpServer.address().port}`,
    origin,
    tokenFile,
    calls,
    gateway,
  };
}

describe("Vite against a real HTTP gateway", () => {
  it("carries live RPC bytes locally, preserves gateway auth and releases the upstream on close", async () => {
    const f = await fixture({ streamRPC: true });
    const socket = new WebSocket(
      f.url.replace("http:", "ws:") + "/rpc-stream/cxz.SessionService/Events",
      { headers: { Origin: f.url, Cookie: "test-session=valid" } },
    );
    const frames = [];
    socket.on("message", (data, binary) => frames.push({ data, binary }));
    await new Promise((resolve, reject) => {
      socket.once("open", resolve);
      socket.once("error", reject);
    });
    const body = Buffer.from([0, 0, 0, 0, 2, 123, 125]);
    socket.send(
      JSON.stringify({
        headers: {
          "content-type": "application/connect+json",
          "connect-protocol-version": "1",
          cookie: "foreign-cookie",
          origin: "https://foreign.test",
          authorization: "ignored",
        },
      }),
    );
    socket.send(body);
    await expect.poll(() => frames.length).toBe(2);
    expect(JSON.parse(frames[0].data.toString()).status).toBe(200);
    expect(frames[1].binary).toBe(true);
    expect(frames[1].data).toEqual(body);
    expect(f.calls[0].headers.origin).toBe(f.origin);
    expect(f.calls[0].headers.cookie).toBe("test-session=valid");
    expect(f.calls[0].headers.authorization).toBeUndefined();
    expect(f.calls[0].body).toBe(body.toString());
    socket.close();
    await expect
      .poll(() => f.calls.some((call) => call.path === "stream canceled"))
      .toBe(true);
    const wrong = new WebSocket(
      f.url.replace("http:", "ws:") + "/rpc-stream/cxz.SessionService/Events",
      {
        headers: {
          Origin: "https://foreign.test",
          Cookie: "test-session=valid",
        },
      },
    );
    await new Promise((resolve) => wrong.once("error", resolve));
    expect(f.calls).toHaveLength(2);
  });
  it("does not expose a token through Vite's public directory", async () => {
    const f = await fixture({ publicToken: true });
    for (const path of [
      "/web-token",
      "/web-token?raw",
      "/%77eb-token",
      `/@fs/${f.tokenFile}?raw`,
    ]) {
      const r = await fetch(f.url + path);
      expect(r.status).toBe(403);
      expect(await r.text()).not.toContain(token);
    }
  });
  it("does not serve the token even when it is inside Vite's project root", async () => {
    const f = await fixture();
    for (const path of [
      "/web-token",
      "/web-token?raw",
      `/@fs/${f.tokenFile}`,
      `/@fs/${f.tokenFile}?raw`,
    ]) {
      const r = await fetch(f.url + path);
      expect(r.status).toBe(403);
      expect(await r.text()).not.toContain(token);
    }
  });
  it("signs in server-side and supplies a cache identity for older gateways without exposing the token", async () => {
    const f = await fixture();
    const r = await fetch(`${f.url}/auth/status`);
    expect(r.status).toBe(204);
    expect(r.headers.get("set-cookie")).toContain("HttpOnly");
    expect(r.headers.get("cache-control")).toBe("no-store");
    expect(r.headers.get("payday-identity")).toMatch(/^[0-9a-f]{32}$/);
    expect(await r.text()).not.toContain(token);
    expect(JSON.stringify([...r.headers])).not.toContain(token);
    expect(f.calls.map((c) => c.path)).toEqual(["/auth/status", "/auth/login"]);
    expect(JSON.parse(f.calls[1].body)).toEqual({ token });
    expect(f.calls[1].headers.origin).toBe(f.origin);
    expect(f.calls[1].headers.host).toBe(new URL(f.origin).host);
  });

  it("proxies raw attachment bytes and rewrites the dev origin", async () => {
    const f = await fixture();
    const body = new Uint8Array([0, 255, 13, 10]);
    const response = await fetch(
      `${f.url}/attachments/session?run=run&name=report.bin&size=4`,
      {
        method: "POST",
        headers: { Origin: f.url, "Content-Type": "application/octet-stream" },
        body,
      },
    );
    expect(response.status).toBe(200);
    expect(await response.text()).toBe("proxied");
    expect(f.calls[0].path).toBe(
      "/attachments/session?run=run&name=report.bin&size=4",
    );
    expect(f.calls[0].headers.origin).toBe(f.origin);
    expect(f.calls[0].headers["content-type"]).toBe("application/octet-stream");
  });

  it("reuses an authenticated cookie without creating another session", async () => {
    const f = await fixture();
    const first = await fetch(`${f.url}/auth/status`);
    f.calls.splice(0);
    const r = await fetch(`${f.url}/auth/status`, {
      headers: { Cookie: "test-session=valid" },
    });
    expect(r.status).toBe(204);
    expect(r.headers.get("set-cookie")).toBeNull();
    expect(r.headers.get("payday-identity")).toBe(
      first.headers.get("payday-identity"),
    );
    expect(f.calls).toHaveLength(1);
  });

  it("preserves the authenticated identity returned by an updated gateway", async () => {
    const f = await fixture({ identity: "gateway-authority" });
    for (const headers of [{}, { Cookie: "test-session=valid" }]) {
      const response = await fetch(`${f.url}/auth/status`, { headers });
      expect(response.status).toBe(204);
      expect(response.headers.get("payday-identity")).toBe("gateway-authority");
    }
  });

  it("reads a rotated token on the next sign-in", async () => {
    const f = await fixture();
    const rotated = "rotated-test-token-".repeat(3);
    await writeFile(f.tokenFile, rotated);
    await fetch(`${f.url}/auth/status`);
    expect(JSON.parse(f.calls[1].body).token).toBe(rotated);
  });

  it("keeps error responses free of token contents", async () => {
    const f = await fixture({ loginStatus: 403 });
    const r = await fetch(`${f.url}/auth/status`);
    expect(r.status).toBe(403);
    expect(r.headers.get("payday-identity")).toBeNull();
    expect(await r.text()).not.toContain(token);
  });

  it("proxies RPCs and logout with the gateway's required Host and Origin", async () => {
    const f = await fixture();
    for (const path of [
      "/cxz.SessionService/Events",
      "/payday.BatchService/Do",
      "/auth/logout",
      "/editor/project/file",
    ]) {
      const r = await fetch(f.url + path, {
        method: "POST",
        headers: { Origin: f.url, Cookie: "test-session=valid" },
        body: "request",
      });
      expect(await r.text()).toBe("proxied");
    }
    expect(f.calls).toHaveLength(4);
    for (const c of f.calls) {
      expect(c.headers.origin).toBe(f.origin);
      expect(c.headers.host).toBe(new URL(f.origin).host);
      expect(c.headers.cookie).toBe("test-session=valid");
      expect(c.body).toBe("request");
    }
  });

  it("rejects cross-origin login and mutation requests before they reach cxz", async () => {
    const f = await fixture();
    for (const [path, options] of [
      ["/auth/status", { headers: { Origin: "https://other.example" } }],
      ["/auth/status", { headers: { "Sec-Fetch-Site": "cross-site" } }],
      ["/cxz.SessionService/Send", { method: "POST" }],
      [
        "/cxz.SessionService/Send",
        { method: "POST", headers: { Origin: "https://other.example" } },
      ],
    ]) {
      expect((await fetch(f.url + path, options)).status).toBe(403);
    }
    expect(f.calls).toHaveLength(0);
  });

  it("forwards terminal WebSocket upgrades and rejects another origin", async () => {
    const f = await fixture();
    const upgrades = [];
    f.gateway.on("upgrade", (req, socket) => {
      upgrades.push(req.headers);
      socket.end(
        "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n",
      );
    });
    const upgrade = (origin) =>
      new Promise((resolve) => {
        const req = request(`${f.url}/terminal/project?cols=80&rows=24`, {
          headers: {
            Connection: "Upgrade",
            Upgrade: "websocket",
            Origin: origin,
            Cookie: "test-session=valid",
          },
        });
        req.on("upgrade", (_, socket) => {
          socket.destroy();
          resolve(true);
        });
        req.on("error", () => resolve(false));
        req.end();
      });
    expect(await upgrade(f.url)).toBe(true);
    expect(upgrades[0].origin).toBe(f.origin);
    expect(upgrades[0].cookie).toBe("test-session=valid");
    expect(await upgrade("https://other.example")).toBe(false);
  });
});

// A gateway of one installation for the hub tests: its session cookie is its
// own name, so which gateway a request reached with whose session is visible.
async function hubGateway(dir, name) {
  const tokenFile = join(dir, `${name}-token`);
  await writeFile(tokenFile, token);
  const calls = [];
  const session = `__Host-cxz=${name}-session`;
  const server = createHttpServer(async (req, res) => {
    const parts = [];
    for await (const part of req) parts.push(part);
    calls.push({
      path: req.url,
      headers: req.headers,
      body: Buffer.concat(parts).toString(),
    });
    if (req.url === "/auth/status") {
      res.writeHead(req.headers.cookie === session ? 204 : 401);
      res.end();
    } else if (req.url === "/auth/login") {
      res.writeHead(204, {
        "Set-Cookie": `${session}; Path=/; Secure; HttpOnly; SameSite=Strict`,
        "Payday-Identity": `${name}-identity`,
      });
      res.end();
    } else if (req.url === "/auth/logout") {
      res.writeHead(204, { "Set-Cookie": "__Host-cxz=; Path=/; Max-Age=-1" });
      res.end();
    } else if (req.url === "/cxz.SessionService/Events") {
      res.writeHead(200, { "Content-Type": "application/connect+json" });
      res.end(Buffer.from([0, 0, 0, 0, 2, 123, 125]));
    } else {
      res.end(`${name} answered`);
    }
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  cleanup.push(
    () =>
      new Promise((resolve) => {
        server.close(resolve);
        server.closeAllConnections();
      }),
  );
  return {
    name,
    origin: `http://127.0.0.1:${server.address().port}`,
    tokenFile,
    calls,
    server,
    session,
  };
}

async function hubFixture() {
  const dir = await temp();
  const dev = await hubGateway(dir, "dev");
  const k8s = await hubGateway(dir, "k8s");
  const vite = await createServer({
    root: dir,
    configFile: false,
    plugins: [devGatewayPlugin([dev, k8s])],
    logLevel: "silent",
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { host: "127.0.0.1", port: 0, watch: null },
  });
  await vite.listen();
  cleanup.push(() => vite.close());
  return {
    url: `http://127.0.0.1:${vite.httpServer.address().port}`,
    dev,
    k8s,
  };
}

describe("Vite as a hub of connections", () => {
  it("reads named connections and refuses bad names", async () => {
    const dir = await temp();
    await config(join(dir, "dev"));
    await config(join(dir, "k8s"), "web.json", {
      origin: "https://alice.cxz.example",
    });
    const file = join(dir, "connections.json");
    await writeFile(
      file,
      JSON.stringify({
        connections: [
          { name: "dev", config: "dev/web.json" },
          { name: "k8s", config: "k8s/web.json" },
        ],
      }),
    );
    const got = await loadDevConnections({ CXZ_WEB_CONNECTIONS: file });
    expect(got.map((c) => [c.name, c.origin])).toEqual([
      ["dev", "http://127.0.0.1:7350"],
      ["k8s", "https://alice.cxz.example"],
    ]);
    await writeFile(
      file,
      JSON.stringify({
        connections: [
          { name: "renamed", config: "k8s/web.json" },
          { name: "dev", config: "dev/web.json" },
        ],
      }),
    );
    const reordered = await loadDevConnections({ CXZ_WEB_CONNECTIONS: file });
    expect(reordered.map((c) => c.id)).toEqual([got[1].id, got[0].id]);
    expect(got[1].tokenFile).toBe(join(dir, "k8s", "web-token"));
    for (const connections of [
      [{ name: "K8s", config: "k8s/web.json" }],
      [
        { name: "a", config: "dev/web.json" },
        { name: "a", config: "k8s/web.json" },
      ],
      [],
    ]) {
      await writeFile(file, JSON.stringify({ connections }));
      await expect(
        loadDevConnections({ CXZ_WEB_CONNECTIONS: file }),
      ).rejects.toThrow();
    }
  });

  it("keeps each connection's session server-side and sends it only to its gateway", async () => {
    const f = await hubFixture();
    const status = await fetch(f.url + "/c/@k8s/auth/status", {
      headers: { Origin: f.url },
    });
    expect(status.status).toBe(204);
    expect(status.headers.getSetCookie()).toEqual([]);
    expect(status.headers.get("payday-identity")).toBe("k8s-identity");
    const list = await fetch(f.url + "/c/@k8s/cxz.ProjectService/List", {
      method: "POST",
      headers: {
        Origin: f.url,
        Cookie: "__Host-cxz=browser-cookie",
        "Content-Type": "application/json",
      },
      body: "{}",
    });
    expect(await list.text()).toBe("k8s answered");
    const call = f.k8s.calls.at(-1);
    expect(call.path).toBe("/cxz.ProjectService/List");
    expect(call.headers.cookie).toBe(f.k8s.session);
    expect(call.headers.origin).toBe(f.k8s.origin);
    expect(f.dev.calls).toHaveLength(0);
    const { items } = await (
      await fetch(f.url + "/cxz.ConnectionService/List", {
        method: "POST",
        headers: { Origin: f.url, "Content-Type": "application/json" },
        body: "{}",
      })
    ).json();
    const id = Buffer.from(items[1].id, "base64")
      .toString("hex")
      .replace(/^(\w{8})(\w{4})(\w{4})(\w{4})(\w{12})$/, "$1-$2-$3-$4-$5")
      .toUpperCase();
    const byId = await fetch(f.url + `/c/${id}/cxz.ProjectService/List`, {
      method: "POST",
      headers: { Origin: f.url },
      body: "{}",
    });
    expect(await byId.text()).toBe("k8s answered");
    expect(f.k8s.calls.at(-1).headers.cookie).toBe(f.k8s.session);
    expect(f.dev.calls).toHaveLength(0);
    // Unprefixed is the default connection, as a single gateway was.
    expect(
      await (
        await fetch(f.url + "/cxz.ProjectService/List", {
          method: "POST",
          headers: { Origin: f.url },
          body: "{}",
        })
      ).text(),
    ).toBe("dev answered");
    // Signing out of one connection forgets its session.
    await fetch(f.url + "/c/@k8s/auth/logout", {
      method: "POST",
      headers: { Origin: f.url },
    });
    await fetch(f.url + "/c/@k8s/cxz.ProjectService/List", {
      method: "POST",
      headers: { Origin: f.url },
      body: "{}",
    });
    expect(f.k8s.calls.at(-1).headers.cookie).toBeUndefined();
  });

  it("answers ConnectionService.List itself and refuses unknown connections", async () => {
    const f = await hubFixture();
    for (const path of [
      "/cxz.ConnectionService/List",
      "/c/@k8s/cxz.ConnectionService/List",
    ]) {
      const r = await fetch(f.url + path, {
        method: "POST",
        headers: { Origin: f.url, "Content-Type": "application/json" },
        body: "{}",
      });
      expect(r.status).toBe(200);
      expect(await r.json()).toEqual({
        items: [
          {
            id: expect.any(String),
            name: "dev",
            target: f.dev.origin,
            transport: "gateway",
            isDefault: true,
            readOnly: true,
          },
          {
            id: expect.any(String),
            name: "k8s",
            target: f.k8s.origin,
            transport: "gateway",
            isDefault: false,
            readOnly: true,
          },
        ],
      });
    }
    const proto = await fetch(f.url + "/cxz.ConnectionService/List", {
      method: "POST",
      headers: { Origin: f.url, "Content-Type": "application/proto" },
      body: "",
    });
    expect(proto.status).toBe(415);
    expect(
      (
        await fetch(f.url + "/c/@nope/cxz.ProjectService/List", {
          method: "POST",
          headers: { Origin: f.url },
          body: "{}",
        })
      ).status,
    ).toBe(404);
    expect(
      (
        await fetch(f.url + "/c/@k8s/cxz.ProjectService/List", {
          method: "POST",
          headers: { Origin: "https://other.example" },
          body: "{}",
        })
      ).status,
    ).toBe(403);
    expect(f.dev.calls.length + f.k8s.calls.length).toBe(0);
  });

  it("carries a connection's streams and sockets with its session", async () => {
    const f = await hubFixture();
    await fetch(f.url + "/c/@k8s/auth/status", { headers: { Origin: f.url } });
    const socket = new WebSocket(
      f.url.replace("http:", "ws:") +
        "/c/@k8s/rpc-stream/cxz.SessionService/Events",
      {
        headers: { Origin: f.url, Cookie: "__Host-cxz=browser-cookie" },
      },
    );
    const frames = [];
    socket.on("message", (data) => frames.push(data));
    await new Promise((resolve, reject) => {
      socket.once("open", resolve);
      socket.once("error", reject);
    });
    socket.send(
      JSON.stringify({
        headers: { "content-type": "application/connect+json" },
      }),
    );
    socket.send(Buffer.from([0, 0, 0, 0, 2, 123, 125]));
    await expect.poll(() => frames.length).toBeGreaterThanOrEqual(2);
    socket.close();
    const stream = f.k8s.calls.find(
      (c) => c.path === "/cxz.SessionService/Events",
    );
    expect(stream.headers.cookie).toBe(f.k8s.session);
    expect(stream.headers.origin).toBe(f.k8s.origin);

    const upgrades = [];
    f.k8s.server.on("upgrade", (req, s) => {
      upgrades.push(req);
      s.end(
        "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n",
      );
    });
    const opened = await new Promise((resolve) => {
      const req = request(`${f.url}/c/@k8s/terminal/project?cols=80&rows=24`, {
        headers: {
          Connection: "Upgrade",
          Upgrade: "websocket",
          Origin: f.url,
          Cookie: "__Host-cxz=browser-cookie",
        },
      });
      req.on("upgrade", (_, s) => {
        s.destroy();
        resolve(true);
      });
      req.on("error", () => resolve(false));
      req.end();
    });
    expect(opened).toBe(true);
    expect(upgrades[0].url).toBe("/terminal/project?cols=80&rows=24");
    expect(upgrades[0].headers.cookie).toBe(f.k8s.session);
    expect(upgrades[0].headers.origin).toBe(f.k8s.origin);
  });
});
