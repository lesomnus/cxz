import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { isIP } from "node:net";
import { homedir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { devRPCStreams } from "./rpc-stream.mjs";

const routes = [
  "/auth/",
  "/cxz.",
  "/payday.",
  "/editor/",
  "/terminal/",
  "/login/",
  "/attachments/",
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
    let raw;
    try {
      raw = await readFile(path, "utf8");
    } catch (error) {
      if (error.code === "ENOENT") continue;
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
  throw new Error(
    "No cxz web configuration found. Run cxz web up on this host first, or set CXZ_WEB_CONFIG.",
  );
}

// A Host header this server answers to. Loopback and a literal address are
// taken as they are: an address cannot be re-resolved, so a page served from
// one was served by this machine. A *name* can be pointed here by somebody
// else's DNS, which is what this check is against, so a name has to be one the
// operator already listed in Vite's `server.allowedHosts` -- the same rule
// Vite applies to the page itself, rather than a second list to keep.
function servedHost(hostHeader, port, allowedHosts) {
  if (typeof hostHeader !== "string") return false;
  const host = hostHeader.trim();
  let name, rest;
  if (host.startsWith("[")) {
    const end = host.indexOf("]");
    if (end < 0) return false;
    name = host.slice(1, end);
    rest = host.slice(end + 1);
    if (!isIP(name)) return false;
  } else {
    const colon = host.indexOf(":");
    name = colon === -1 ? host : host.slice(0, colon);
    rest = colon === -1 ? "" : host.slice(colon);
  }
  if (rest !== `:${port}`) return false;
  if (name === "localhost" || name.endsWith(".localhost")) return true;
  if (isIP(name)) return true;
  if (allowedHosts === true) return true;
  return Array.isArray(allowedHosts) && allowedHosts.includes(name);
}

// A browser only keeps a `Secure` cookie on a secure origin, and a browser that
// reached a plain-HTTP dev server by address is not on one -- only loopback
// counts as trustworthy. The gateway marks its session cookie `Secure`, rightly,
// so forwarding it unchanged to a workstation on the network hands over a cookie
// that browser throws away and a login screen that never clears. This takes the
// flag off on the way through, and only there: the gateway's own answer, and
// every cookie it sets on a loopback page, is untouched.
function forwardCookies(cookies, secure) {
  if (secure) return cookies;
  return cookies.map((cookie) =>
    cookie
      .split(";")
      .filter((part) => part.trim().toLowerCase() !== "secure")
      .join(";"),
  );
}

// Whether the page this answer goes to is a secure context, which over plain
// HTTP is loopback and nothing else.
function secureContext(hostHeader) {
  if (typeof hostHeader !== "string") return false;
  const host = hostHeader.trim();
  const name = host.startsWith("[")
    ? host.slice(1, host.indexOf("]"))
    : host.split(":")[0];
  return (
    name === "localhost" ||
    name.endsWith(".localhost") ||
    name === "127.0.0.1" ||
    name === "::1"
  );
}

function localRequest(req, port, allowedHosts, upgrade = false) {
  const host = req.headers.host;
  if (!servedHost(host, port, allowedHosts)) return false;
  if (req.headers["sec-fetch-site"] === "cross-site") return false;
  const origin = req.headers.origin;
  if (origin !== undefined) return origin === `http://${host.trim()}`;
  return !upgrade && (req.method === "GET" || req.method === "HEAD");
}

export function devGatewayPlugin(gateway) {
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
              gateway.tokenFile
                .replaceAll("\\", "/")
                .replace(/[?*\[\]{}()!+@]/g, "\\$&"),
            ],
          },
          proxy: Object.fromEntries(
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
        },
      };
    },
    configureServer(server) {
      if (server.config.server.https) {
        throw new Error(
          "cxz automatic dev sign-in requires an HTTP Vite server.",
        );
      }
      const host = server.config.server.host;
      const allowedHosts = server.config.server.allowedHosts;
      const port = () => server.httpServer.address().port;
      const local = (req, upgrade = false) =>
        localRequest(req, port(), allowedHosts, upgrade);
      devRPCStreams(server, gateway, (req) => local(req, true));
      const publicRelative = server.config.publicDir
        ? relative(server.config.publicDir, gateway.tokenFile)
        : "..";
      const publicTokenPath =
        !publicRelative.startsWith(`..${sep}`) && publicRelative !== ".."
          ? "/" + publicRelative.replaceAll("\\", "/")
          : undefined;
      const loopback =
        host === undefined ||
        host === true ||
        host === "127.0.0.1" ||
        host === "localhost" ||
        host === "::1";
      server.config.logger.info(
        `cxz dev: ${gateway.origin} (automatic local sign-in)`,
      );
      if (!loopback) {
        // Said out loud on every start, because the thing that makes this
        // server convenient is the thing being handed out: a browser that
        // reaches it is signed in as the installation owner, with no
        // credential of its own.
        server.config.logger.warn(
          `cxz dev: reachable on ${host}; anybody who can reach port ${server.config.server.port} is signed in as the owner`,
        );
      }

      // Origin rewriting is for the gateway's exact-origin binding. Validate
      // browser requests before rewriting so another site cannot use this login.
      const checkUpgrade = (req, socket) => {
        if (isGatewayPath(req.url || "") && !local(req, true)) {
          socket.destroy();
        }
      };
      server.httpServer.prependListener("upgrade", checkUpgrade);
      server.httpServer.once("close", () =>
        server.httpServer.removeListener("upgrade", checkUpgrade),
      );
      server.middlewares.use((req, res, next) => {
        // Vite's public directory bypasses fs.deny, unlike source and /@fs/.
        if (publicTokenPath) {
          let path;
          try {
            path = decodeURIComponent(new URL(req.url, "http://dev").pathname);
          } catch {
            return next();
          }
          if (path === publicTokenPath) {
            res.writeHead(403);
            res.end("Access denied");
            return;
          }
        }
        if (!isGatewayPath(req.url || "")) return next();
        if (!local(req)) {
          res.writeHead(403);
          res.end("Local dev origin required");
          return;
        }
        if (req.method !== "GET" || req.url !== "/auth/status") return next();
        void signIn(req, res, gateway);
      });
    },
  };
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
      res.setHeader(
        "Set-Cookie",
        forwardCookies(
          login.headers.getSetCookie(),
          secureContext(req.headers.host),
        ),
      );
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
