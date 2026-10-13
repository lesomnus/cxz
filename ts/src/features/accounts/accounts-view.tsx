import { useRef, useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { AgentIdentity } from "#src/shared/components/agent-identity.tsx";
import { listAccounts } from "./account-data";
import { AccountAddForm } from "./account-add-form";
import { AccountDetails } from "./account-details";
import resourceStyles from "#src/shared/components/resource-surface.module.css";
import styles from "./accounts.module.css";
export function AccountsView({
  c,
  selected,
  select,
  sessions = [],
  sessionsLoading = false,
}: {
  c: Connection;
  selected?: string;
  select?: (alias: string) => void;
  sessions?: Session[];
  sessionsLoading?: boolean;
}) {
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const [chosen, setChosen] = useState(""),
    [adding, setAdding] = useState(false);
  const panel = useRef<HTMLElement>(null);
  const alias =
    (selected !== undefined ? selected : chosen) ||
    accounts.data?.[0]?.alias ||
    "";
  const account = accounts.data?.find((account) => account.alias === alias);
  const choose = (alias: string) => {
    setChosen(alias);
    select?.(alias);
  };
  return (
    <>
      <aside
        ref={panel}
        className={`resource-panel ${styles.panel}`}
        aria-label="Account list"
      >
        <header className={styles.header}>
          <strong>Accounts</strong>
          <Button
            type="button"
            className={styles.add}
            aria-label="Add account"
            title="Add account"
            onClick={() => setAdding(true)}
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              aria-hidden="true"
            >
              <path d="M12 5v14M5 12h14" />
            </svg>
          </Button>
        </header>
        <PanelScroll>
          {accounts.data?.map((account) => (
            <Button
              key={account.alias}
              className={styles.item}
              pressTarget=".session-heading"
              aria-pressed={account.alias === alias}
              onClick={() => choose(account.alias)}
            >
              <AgentIdentity
                agent={account.agent}
                name={account.name || account.alias}
                alias={account.alias}
              />
            </Button>
          ))}
          {accounts.loading && !accounts.data && (
            <p className={resourceStyles.muted}>Loading accounts…</p>
          )}
          {accounts.error && <p role="alert">{accounts.error}</p>}
          {!accounts.loading && !accounts.data?.length && (
            <p className={resourceStyles.muted}>No accounts yet.</p>
          )}
        </PanelScroll>
      </aside>
      <main className={resourceStyles.page}>
        <div className={resourceStyles.body}>
          {account ? (
            <AccountDetails
              key={account.alias}
              c={c}
              account={account}
              sessions={sessions.filter(
                (session) => session.account?.alias === account.alias,
              )}
              sessionsLoading={sessionsLoading}
            />
          ) : (
            <p className={resourceStyles.muted}>
              {accounts.loading
                ? "Loading account…"
                : alias
                  ? "Account not found. Choose another account."
                  : "Select an account, or add one using the panel."}
            </p>
          )}
        </div>
      </main>
      {adding && (
        <AccountAddForm
          c={c}
          anchor={panel.current}
          close={() => setAdding(false)}
          added={(account) => {
            setAdding(false);
            accounts.reload();
            choose(account.alias);
          }}
        />
      )}
    </>
  );
}
