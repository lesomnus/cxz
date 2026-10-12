import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { LoginPanel } from "./login-panel";
import styles from "#src/shared/components/resource-surface.module.css";

export async function listAccounts(c: Connection, signal?: AbortSignal) {
  const items = [];
  let after = "";
  do {
    const page = await c.accounts.list(
      { size: 200, after },
      { signal, timeoutMs: 10000 },
    );
    items.push(...page.items);
    after = page.next;
  } while (after);
  return items;
}
export function AccountList({ c }: { c: Connection }) {
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const [agent, setAgent] = useState("codex");
  const [alias, setAlias] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [login, setLogin] = useState("");
  return (
    <div className={styles.content}>
      <section className={styles.section}>
        <strong>New account</strong>
        <ValueMenu
          label="Agent"
          value={agent}
          options={[
            { value: "codex", label: "Codex" },
            { value: "claude", label: "Claude" },
          ]}
          choose={setAgent}
        />
        <label className={styles.field}>
          Alias
          <input
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            maxLength={63}
          />
        </label>
        <label className={styles.field}>
          Display name
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <Button
          disabled={busy || !alias.trim()}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              await c.accounts.add(
                { agent, alias: alias.trim(), name },
                { timeoutMs: 30000 },
              );
              setAlias("");
              setName("");
              accounts.reload();
            } catch (error) {
              setError(String(error));
            } finally {
              setBusy(false);
            }
          }}
        >
          Add account
        </Button>
      </section>
      {(error || accounts.error) && (
        <p role="alert">{error || accounts.error}</p>
      )}
      {accounts.loading && <p className={styles.muted}>Loading accounts…</p>}
      {(accounts.data ?? []).map((account) => (
        <section key={account.alias} className={styles.section}>
          <div className={styles.row}>
            <div>
              <strong>{account.name || account.alias}</strong>
              <div className={styles.status}>
                {account.alias} · {account.agent} · {account.authBackend}
              </div>
            </div>
            {account.authBackend === "brokered-access-token" && (
              <Button onClick={() => setLogin(account.alias)}>Log in</Button>
            )}
          </div>
          {account.authBackend === "project-local-oauth" && (
            <small className={styles.muted}>
              Each session has its own login. Choose this account when creating
              a session, or log in from its details after stopping it.
            </small>
          )}
        </section>
      ))}
      {!accounts.loading && !accounts.data?.length && (
        <p className={styles.muted}>No accounts yet.</p>
      )}
      {login && (
        <LoginPanel
          key={login}
          c={c}
          target={{ account: login }}
          complete={() => {
            setLogin("");
            accounts.reload();
          }}
          cancel={() => setLogin("")}
        />
      )}
    </div>
  );
}
