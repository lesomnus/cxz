import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";

const routes = ["/auth/", "/cxz.", "/payday.", "/editor/", "/terminal/"];
const isGatewayPath = (path) => routes.some((route) => path.startsWith(route));

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

function localRequest(req, port, upgrade = false) {
  const host = req.headers.host;
  if (host !== `127.0.0.1:${port}` && host !== `localhost:${port}`)
    return false;
  if (req.headers["sec-fetch-site"] === "cross-site") return false;
  const origin = req.headers.origin;
  if (origin !== undefined) return origin === `http://${host}`;
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
      if (
        server.config.server.host !== "127.0.0.1" ||
        server.config.server.https
      ) {
        throw new Error(
          "cxz automatic dev sign-in requires a loopback HTTP Vite server.",
        );
      }
      const port = () => server.httpServer.address().port;
      const publicRelative = server.config.publicDir
        ? relative(server.config.publicDir, gateway.tokenFile)
        : "..";
      const publicTokenPath =
        !publicRelative.startsWith(`..${sep}`) && publicRelative !== ".."
          ? "/" + publicRelative.replaceAll("\\", "/")
          : undefined;
      server.config.logger.info(
        `cxz dev: ${gateway.origin} (automatic local sign-in)`,
      );

      // Origin rewriting is for the gateway's exact-origin binding. Validate
      // browser requests before rewriting so another site cannot use this login.
      const checkUpgrade = (req, socket) => {
        if (isGatewayPath(req.url || "") && !localRequest(req, port(), true)) {
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
        if (!localRequest(req, port())) {
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
      res.writeHead(status.status);
      res.end(
        status.ok ? undefined : `cxz gateway returned HTTP ${status.status}`,
      );
      return;
    }
    // The access token never enters a browser response, Vite's source graph or
    // client storage. Only the gateway's HttpOnly session cookie is forwarded.
    const login = await fetch(`${gateway.origin}/auth/login`, {
      method: "POST",
      headers: { Origin: gateway.origin, "Content-Type": "application/json" },
      body: JSON.stringify({ token: await readToken(gateway.tokenFile) }),
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    await login.arrayBuffer();
    if (login.ok) {
      res.setHeader("Set-Cookie", login.headers.getSetCookie());
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
