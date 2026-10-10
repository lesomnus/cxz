import { t } from "../shared/i18n/i18n";
import { useLocale } from "../shared/i18n/i18n-react";
import React, { type PropsWithChildren, useEffect, useState } from "react";
import { Provider } from "@lesomnus/payday/react";
import { Connection, authenticate } from "../shared/api/connection";
import { Button } from "@lesomnus/cxz-ui";
import { Workspace } from "./workspace-shell";
import "./styles/style.css";
export function App({ children }: PropsWithChildren) {
  useLocale();
  const [connection, setConnection] = useState<Connection>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    fetch("/auth/status")
      .then((r) => {
        if (r.ok) setConnection(new Connection());
      })
      .catch(() => setError(t("Cannot reach cxz")));
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
    setConnection(undefined);
  }
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
