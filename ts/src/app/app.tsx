import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  type FormEvent,
  type PropsWithChildren,
  useEffect,
  useRef,
  useState,
} from "react";
import { Provider } from "@lesomnus/payday/react";
import { Connection, authenticate } from "#src/shared/api/connection.ts";
import { Button } from "@lesomnus/nox";
import { Workspace } from "./workspace-shell";
import "#src/app/styles/style.css";
import { useRouteConnections, useWorkspaceRoute } from "./router";
import { LogsPage } from "#src/pages/logs/logs-page.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { recordClientError } from "#src/shared/logs/client-logs.ts";
import {
  ConnectionScope,
  resourceLocation,
} from "#src/shared/navigation/resource-route.ts";
import {
  defaultConnection,
  resolveConnection,
  type RoutableConnection,
} from "#src/shared/api/routable-connections.ts";
import { NotFoundPage } from "#src/pages/not-found/not-found-page.tsx";

export function App({ children }: PropsWithChildren) {
  const route = useWorkspaceRoute();
  const entries = useRouteConnections();
  const last = useRef<string>(undefined);
  const target = route.connection
    ? resolveConnection(entries, route.connection)
    : ((last.current ? resolveConnection(entries, last.current) : undefined) ??
      defaultConnection(entries));
  if (!target) return <NotFoundPage />;
  if (route.connection) last.current = target.id ?? target.handle;
  return (
    <ConnectionScope value={target.handle}>
      <Installation key={target.id ?? target.handle} target={target}>
        {children}
      </Installation>
    </ConnectionScope>
  );
}

function Installation({
  target,
  children,
}: PropsWithChildren<{ target: RoutableConnection }>) {
  useLocale();
  const route = useWorkspaceRoute();
  const [connection, setConnection] = useState<Connection>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const { baseUrl } = target;
  useEffect(() => {
    const request = new AbortController();
    fetch(`${baseUrl}/auth/status`, {
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(10000)]),
    })
      .then((r) => {
        if (request.signal.aborted || r.status === 401) return;
        if (!r.ok || r.headers.get("content-type")?.includes("text/html"))
          throw new Error(t("Cannot reach cxz"));
        setConnection(
          new Connection(
            baseUrl,
            undefined,
            undefined,
            r.headers.get("payday-identity") ?? undefined,
          ),
        );
      })
      .catch((error: unknown) => {
        if (request.signal.aborted) return;
        recordClientError(error, "Cannot reach cxz");
        setError(t("Cannot reach cxz"));
      });
    return () => request.abort();
  }, [baseUrl]);
  useEffect(() => () => connection?.dispose(), [connection]);

  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const identity = await authenticate(token, baseUrl);
      setToken("");
      setConnection(new Connection(baseUrl, undefined, undefined, identity));
      setError("");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function logout() {
    const r = await fetch(`${baseUrl}/auth/logout`, { method: "POST" });
    if (!r.ok) throw Error(t("Sign out failed"));
    connection?.drafts.clear();
    connection?.attention.clear();
    connection?.selection.clear();
    await connection?.forgetCachedTasks();
    setConnection(undefined);
  }
  if (!connection && route.resource === "logs")
    return (
      <div
        style={{ height: "100dvh", display: "flex", flexDirection: "column" }}
      >
        <RouteLink {...resourceLocation("sessions", target.handle)}>
          {t("Back to sessions")}
        </RouteLink>
        <LogsPage />
      </div>
    );
  if (!connection)
    return (
      <main className="login">
        <h1>cxz</h1>
        <p>{t("Your projects, wherever you are.")}</p>
        <form onSubmit={login}>
          <label>
            {t("Web access token")}
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoComplete="current-password"
              required
            />
          </label>
          <Button disabled={busy}>{t("Connect")}</Button>
        </form>
        <p role="alert">{error}</p>
        <small>{target.handle}</small>
        <RouteLink to="/log">{t("Logs")}</RouteLink>
      </main>
    );
  return (
    <Provider app={connection}>
      <Workspace connection={connection} logout={logout}>
        {children}
      </Workspace>
    </Provider>
  );
}
