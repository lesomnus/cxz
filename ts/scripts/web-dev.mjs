import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { homedir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { devRPCStreams } from "./rpc-stream.mjs";
import {
  answerList,
  connectionName,
  connectionOf,
  connectionPrefix,
  listPath,
  Sessions,
} from "./web-hub.mjs";

const routes = [
  "/auth/",
  "/cxz.",
  "/payday.",
  "/editor/",
  "/terminal/",
  "/login/",
  "/attachments/",
  "/configuration/",
];
const isGatewayPath = (path) =>
  path.startsWith("/rpc-stream/") ||
  routes.some((route) => path.startsWith(route));

async function readToken(path) {
  let bytes;
  try {
    bytes = await readFile(path);
  } catch {
    throw new Error(`Cannot read cxz web token file: ${path}`);
  }
  const token = bytes.toString("utf8").trim();
  if (bytes.length > 4096 || token.length < 32 || /[ \t\r\n\0]/.test(token)) {
    throw new Error(`Invalid cxz web token file: ${path}`);
  }
  return token;
}

// Prefer the installed settings: web up's flag overrides are saved here, while
// web.json may still describe the defaults from before that installation.
export async function loadDevGateway(env = process.env, home = homedir()) {
  const state =
    env.CXZ_STATE ||
    join(env.XDG_STATE_HOME || join(home, ".local", "state"), "cxz");
  const paths = env.CXZ_WEB_CONFIG
    ? [resolve(env.CXZ_WEB_CONFIG)]
    : [join(state, "web-installation.json"), join(state, "web.json")];
  for (const path of paths) {
    const gateway = await readGatewayConfig(path, true);
    if (gateway) return gateway;
  }
  throw new Error(
    "No cxz web configuration found. Run cxz web up on this host first, or set CXZ_WEB_CONFIG.",
  );
}

// loadDevConnections is the hub's configuration: CXZ_WEB_CONNECTIONS names a
// JSON file of {"connections": [{"name": "dev", "config": "web.json"}, ...]},
// each config a cxz web configuration (relative to the file). The first is the
// default connection. Without it, the one gateway loadDevGateway finds is the
// only connection, named "dev".
export async function loadDevConnections(env = process.env, home = homedir()) {
  if (!env.CXZ_WEB_CONNECTIONS) {
    return [{ name: "dev", ...(await loadDevGateway(env, home)) }];
  }
  const path = resolve(env.CXZ_WEB_CONNECTIONS);
  let file;
  try {
    file = JSON.parse(await readFile(path, "utf8"));
  } catch {
    throw new Error(`Cannot read cxz web connections: ${path}`);
  }
  if (!Array.isArray(file?.connections) || file.connections.length === 0) {
    throw new Error(`cxz web connections lists none: ${path}`);
  }
  const out = [];
  for (const entry of file.connections) {
    if (typeof entry?.name !== "string" || !connectionName.test(entry.name)) {
      throw new Error(
        `Invalid connection name in ${path}: lowercase letters, digits and hyphens`,
      );
    }
    if (out.some((c) => c.name === entry.name)) {
      throw new Error(`Connection ${entry.name} is listed twice in ${path}`);
    }
    if (typeof entry.config !== "string" || !entry.config) {
      throw new Error(`Connection ${entry.name} names no config in ${path}`);
    }
    out.push({
      name: entry.name,
      ...(await readGatewayConfig(resolve(dirname(path), entry.config))),
    });
  }
  return out;
}

async function readGatewayConfig(path, optional = false) {
  {
    let raw;
    try {
      raw = await readFile(path, "utf8");
    } catch (error) {
      if (optional && error.code === "ENOENT") return undefined;
      throw new Error(`Cannot read cxz web configuration: ${path}`);
    }
    let config;
    try {
      config = JSON.parse(raw);
    } catch {
      throw new Error(`Invalid cxz web configuration: ${path}`);
    }
    let url;
    try {
      url = new URL(config.origin);
    } catch {
      throw new Error(`Missing or invalid cxz web origin: ${path}`);
    }
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.username ||
      url.password ||
      url.pathname !== "/" ||
      url.search ||
      url.hash
    ) {
      throw new Error(`Invalid cxz web origin: ${path}`);
    }
    if (
      typeof config.access_token_file !== "string" ||
      !config.access_token_file
    ) {
      throw new Error(
        `Missing access_token_file in cxz web configuration: ${path}`,
      );
    }
    const tokenFile = resolve(dirname(path), config.access_token_file);
    await readToken(tokenFile);
    return { origin: url.origin, tokenFile, configFile: path };
  }
}

function localRequest(req, port, upgrade = false) {
  const host = req.headers.host;
  if (host !== `127.0.0.1:${port}` && host !== `localhost:${port}`)
    return false;
  if (req.headers["sec-fetch-site"] === "cross-site") return false;
  const origin = req.headers.origin;
  if (origin !== undefined) return origin === `http://${host}`;
  return !upgrade && (req.method === "GET" || req.method === "HEAD");
}

const escapeRegExp = (text) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// devGatewayPlugin serves the UI against one gateway or, given connections,
// as a hub of several: the first is reached at unprefixed paths exactly as a
// single gateway was, and every connection at /c/<name>/. See web-hub.mjs.
export function devGatewayPlugin(input) {
  const connections = Array.isArray(input)
    ? input
    : [{ name: "dev", ...input }];
  const gateway = connections[0];
  const sessions = new Sessions();
  // A path a gateway answers, once its /c/<name> prefix is taken off.
  const gatewayPath = (url) => {
    const route = connectionOf(url, connections);
    return route !== undefined && isGatewayPath(route.path);
  };
  const hubProxy = (c) => ({
    target: c.origin,
    changeOrigin: true,
    headers: { Origin: c.origin },
    ws: true,
    rewrite: (path) => path.slice(connectionPrefix.length + c.name.length),
    // The connection's session goes upstream; the browser's cookie does not,
    // and the gateway's Set-Cookie stays here.
    configure(proxy) {
      const withSession = (proxyReq) => {
        proxyReq.removeHeader("cookie");
        const cookie = sessions.cookie(c.name);
        if (cookie) proxyReq.setHeader("cookie", cookie);
      };
      proxy.on("proxyReq", withSession);
      proxy.on("proxyReqWs", withSession);
      proxy.on("proxyRes", (proxyRes) => {
        const setCookie = proxyRes.headers["set-cookie"];
        if (setCookie) {
          sessions.take(c.name, setCookie);
          delete proxyRes.headers["set-cookie"];
        }
      });
    },
  });
  return {
    name: "cxz-local-gateway",
    apply: "serve",
    config(config) {
      return {
        server: {
          fs: {
            deny: [
              ...(config.server?.fs?.deny ?? [
                ".env",
                ".env.*",
                "*.{crt,pem}",
                "**/.git/**",
              ]),
              ...connections.map((c) =>
                c.tokenFile
                  .replaceAll("\\", "/")
                  .replace(/[?*\[\]{}()!+@]/g, "\\$&"),
              ),
            ],
          },
          proxy: {
            ...Object.fromEntries(
              routes.map((path) => [
                path,
                {
                  target: gateway.origin,
                  changeOrigin: true,
                  headers: { Origin: gateway.origin },
                  ws: true,
                },
              ]),
            ),
            ...Object.fromEntries(
              connections.map((c) => [
                `^${escapeRegExp(connectionPrefix + c.name)}(${routes.map(escapeRegExp).join("|")})`,
                hubProxy(c),
              ]),
            ),
          },
        },
      };
    },
    configureServer(server) {
      if (
        server.config.server.host !== "127.0.0.1" ||
        server.config.server.https
      ) {
        throw new Error(
          "cxz automatic dev sign-in requires a loopback HTTP Vite server.",
        );
      }
      const port = () => server.httpServer.address().port;
      // A stream is the default connection's with the browser's cookie, or a
      // hub connection's with the session kept here.
      devRPCStreams(
        server,
        (req) => {
          const route = connectionOf(req.url || "", connections);
          if (!route) return undefined;
          return {
            origin: route.connection.origin,
            path: route.path,
            cookie: route.hub
              ? sessions.cookie(route.connection.name)
              : req.headers.cookie || "",
          };
        },
        (req) => localRequest(req, port(), true),
      );
      const publicTokenPaths = connections.flatMap((c) => {
        const publicRelative = server.config.publicDir
          ? relative(server.config.publicDir, c.tokenFile)
          : "..";
        return !publicRelative.startsWith(`..${sep}`) && publicRelative !== ".."
          ? ["/" + publicRelative.replaceAll("\\", "/")]
          : [];
      });
      for (const c of connections) {
        server.config.logger.info(
          `cxz dev: ${connectionPrefix}${c.name} -> ${c.origin}${c === gateway ? " (default)" : ""} (automatic local sign-in)`,
        );
      }

      // Origin rewriting is for the gateway's exact-origin binding. Validate
      // browser requests before rewriting so another site cannot use this login.
      const checkUpgrade = (req, socket) => {
        const url = req.url || "";
        if (url.startsWith(connectionPrefix) && !gatewayPath(url)) {
          socket.destroy();
          return;
        }
        if (gatewayPath(url) && !localRequest(req, port(), true)) {
          socket.destroy();
        }
      };
      server.httpServer.prependListener("upgrade", checkUpgrade);
      server.httpServer.once("close", () =>
        server.httpServer.removeListener("upgrade", checkUpgrade),
      );
      server.middlewares.use((req, res, next) => {
        // Vite's public directory bypasses fs.deny, unlike source and /@fs/.
        if (publicTokenPaths.length) {
          let path;
          try {
            path = decodeURIComponent(new URL(req.url, "http://dev").pathname);
          } catch {
            return next();
          }
          if (publicTokenPaths.includes(path)) {
            res.writeHead(403);
            res.end("Access denied");
            return;
          }
        }
        const url = req.url || "";
        const route = connectionOf(url, connections);
        if (url.startsWith(connectionPrefix) && route === undefined) {
          res.writeHead(404);
          res.end("No such connection");
          return;
        }
        if (route === undefined || !isGatewayPath(route.path)) return next();
        if (!localRequest(req, port())) {
          res.writeHead(403);
          res.end("Local dev origin required");
          return;
        }
        // The hub's own answer, whichever connection it was asked through.
        if (route.path === listPath) return answerList(req, res, connections);
        if (
          req.method === "POST" &&
          route.hub &&
          route.path === "/auth/logout"
        ) {
          sessions.forget(route.connection.name);
          return next();
        }
        if (req.method !== "GET" || route.path !== "/auth/status")
          return next();
        void (route.hub
          ? hubSignIn(res, route.connection, sessions)
          : signIn(req, res, gateway));
      });
    },
  };
}

// hubSignIn is signIn for a hub connection: the same check and login, with the
// session kept in sessions instead of handed to the browser.
async function hubSignIn(res, gateway, sessions) {
  res.setHeader("Cache-Control", "no-store");
  try {
    const status = await fetch(`${gateway.origin}/auth/status`, {
      headers: {
        Origin: gateway.origin,
        Cookie: sessions.cookie(gateway.name),
      },
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    await status.arrayBuffer();
    if (status.status !== 401) {
      await cacheIdentity(res, status, gateway);
      res.writeHead(status.status);
      res.end(
        status.ok ? undefined : `cxz gateway returned HTTP ${status.status}`,
      );
      return;
    }
    sessions.forget(gateway.name);
    const token = await readToken(gateway.tokenFile);
    const login = await fetch(`${gateway.origin}/auth/login`, {
      method: "POST",
      headers: { Origin: gateway.origin, "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    await login.arrayBuffer();
    if (login.ok) {
      sessions.take(gateway.name, login.headers.getSetCookie());
      await cacheIdentity(res, login, gateway, token);
    }
    res.writeHead(login.status);
    res.end(
      login.ok ? undefined : `cxz dev sign-in failed (HTTP ${login.status})`,
    );
  } catch {
    res.writeHead(502);
    res.end(
      `Cannot sign in to cxz connection ${gateway.name}. Check its gateway and token file.`,
    );
  }
}

async function signIn(req, res, gateway) {
  res.setHeader("Cache-Control", "no-store");
  try {
    const status = await fetch(`${gateway.origin}/auth/status`, {
      headers: { Origin: gateway.origin, Cookie: req.headers.cookie || "" },
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    await status.arrayBuffer();
    if (status.status !== 401) {
      await cacheIdentity(res, status, gateway);
      res.writeHead(status.status);
      res.end(
        status.ok ? undefined : `cxz gateway returned HTTP ${status.status}`,
      );
      return;
    }
    // The access token never enters a browser response, Vite's source graph or
    // client storage. Only the gateway's HttpOnly session cookie is forwarded.
    const token = await readToken(gateway.tokenFile);
    const login = await fetch(`${gateway.origin}/auth/login`, {
      method: "POST",
      headers: { Origin: gateway.origin, "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    await login.arrayBuffer();
    if (login.ok) {
      res.setHeader("Set-Cookie", login.headers.getSetCookie());
      await cacheIdentity(res, login, gateway, token);
    }
    res.writeHead(login.status);
    res.end(
      login.ok ? undefined : `cxz dev sign-in failed (HTTP ${login.status})`,
    );
  } catch {
    res.writeHead(502);
    res.end(
      "Cannot sign in to cxz. Check cxz web status and the configured token file.",
    );
  }
}

async function cacheIdentity(res, response, gateway, token) {
  if (!response.ok) return;
  let identity = response.headers.get("payday-identity");
  if (!identity) {
    try {
      const digest = createHash("sha256")
        .update(token ?? (await readToken(gateway.tokenFile)))
        .digest();
      identity = createHash("sha256")
        .update("cxz.web.cache:")
        .update(digest)
        .digest("hex")
        .slice(0, 32);
    } catch {
      return;
    }
  }
  res.setHeader("Payday-Identity", identity);
}
