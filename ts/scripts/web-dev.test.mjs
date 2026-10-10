import { afterEach, describe, expect, it } from "vitest";
import { createServer as createHttpServer, request } from "node:http";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";
import { devGatewayPlugin, loadDevGateway } from "./web-dev.mjs";

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

async function fixture({ loginStatus = 204, publicToken = false } = {}) {
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
      res.writeHead(req.headers.cookie === "test-session=valid" ? 204 : 401);
      res.end();
    } else if (req.url === "/auth/login") {
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
  it("signs in server-side and forwards only the session cookie", async () => {
    const f = await fixture();
    const r = await fetch(`${f.url}/auth/status`);
    expect(r.status).toBe(204);
    expect(r.headers.get("set-cookie")).toContain("HttpOnly");
    expect(r.headers.get("cache-control")).toBe("no-store");
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
    const r = await fetch(`${f.url}/auth/status`, {
      headers: { Cookie: "test-session=valid" },
    });
    expect(r.status).toBe(204);
    expect(r.headers.get("set-cookie")).toBeNull();
    expect(f.calls).toHaveLength(1);
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
