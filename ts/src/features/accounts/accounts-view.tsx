import { useEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
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
  const [adding, setAdding] = useState(false);
  const content = useRef<HTMLDivElement>(null);
  const add = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!selected) return;
    const card = Array.from(
      content.current?.querySelectorAll<HTMLElement>("[data-account]") ?? [],
    ).find((node) => node.dataset.account === selected);
    card?.scrollIntoView({ block: "start" });
  }, [selected, accounts.data]);
  return (
    <>
      <main className={`${resourceStyles.page} ${styles.page}`}>
        <div className={resourceStyles.body}>
          <div ref={content} className={styles.cards}>
            <header className={styles.header}>
              <Button
                ref={add}
                type="button"
                className={styles.add}
                aria-label="Add account"
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
            {accounts.loading && !accounts.data && (
              <p className={resourceStyles.muted}>Loading accounts…</p>
            )}
            {accounts.error && <p role="alert">{accounts.error}</p>}
            {!accounts.loading && !accounts.data?.length && (
              <p className={resourceStyles.muted}>No accounts yet.</p>
            )}
            {accounts.data?.map((account) => (
              <article
                key={account.alias}
                data-account={account.alias}
                aria-label={account.name || account.alias}
                className={styles.card}
              >
                <AccountDetails
                  c={c}
                  account={account}
                  sessions={sessions.filter(
                    (session) => session.account?.alias === account.alias,
                  )}
                  sessionsLoading={sessionsLoading}
                />
              </article>
            ))}
          </div>
        </div>
      </main>
      {adding && (
        <AccountAddForm
          c={c}
          trigger={add.current}
          close={() => setAdding(false)}
          added={(account) => {
            setAdding(false);
            accounts.reload();
            select?.(account.alias);
          }}
        />
      )}
    </>
  );
}
