import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import React, { type PropsWithChildren, useEffect, useState } from "react";
import { Provider } from "@lesomnus/payday/react";
import { Connection, authenticate } from "#src/shared/api/connection.ts";
import { Button } from "@lesomnus/nox";
import { Workspace } from "./workspace-shell";
import "#src/app/styles/style.css";
import { useWorkspaceRoute } from "./router";
import { LogsPage } from "#src/pages/settings/logs-page.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { recordClientError } from "#src/shared/logs/client-logs.ts";
import { resourceLocation } from "#src/shared/navigation/resource-route.ts";
export function App({ children }: PropsWithChildren) {
  useLocale();
  const route = useWorkspaceRoute();
  const [connection, setConnection] = useState<Connection>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    fetch("/auth/status")
      .then((r) => {
        if (r.ok) setConnection(new Connection());
      })
      .catch((error: unknown) => {
        recordClientError(error, "Cannot reach cxz");
        setError(t("Cannot reach cxz"));
      });
  }, []);
  async function login(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await authenticate(token);
      setToken("");
      setConnection(new Connection());
      setError("");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function logout() {
    const r = await fetch("/auth/logout", { method: "POST" });
    if (!r.ok) throw Error(t("Sign out failed"));
    connection?.drafts.clear();
    connection?.attention.clear();
    connection?.dispose();
    setConnection(undefined);
  }
  if (
    !connection &&
    route.resource === "settings" &&
    route.settingsTopic === "logs"
  )
    return (
      <div
        style={{ height: "100dvh", display: "flex", flexDirection: "column" }}
      >
        <RouteLink {...resourceLocation("sessions")}>
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
        <small>{location.origin}</small>
        <RouteLink to="/setting/logs">{t("Logs")}</RouteLink>
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
