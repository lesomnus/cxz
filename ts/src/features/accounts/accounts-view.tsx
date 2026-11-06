import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useRef, useState } from "react";
import { PageToolbar, SectionLayout } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts, sessionsForAccount } from "./account-data";
import { AccountAddForm } from "./account-add-form";
import { AccountDetails } from "./account-details";
import resourceStyles from "#src/shared/components/resource-surface.module.css";
import styles from "./accounts.module.css";
import { matchesResource } from "#src/shared/navigation/resource-route.ts";
import { key } from "@lesomnus/payday/store";
import {
  ACCOUNT_INFO_TTL,
  ACCOUNT_LIST_KEY,
} from "#src/shared/api/read-cache.ts";
export function AccountsView({
  c,
  selected,
  select,
  sessions = [],
  projects = [],
  sessionsLoading = false,
}: {
  c: Connection;
  selected?: string;
  select?: (alias: string) => void;
  sessions?: Session[];
  projects?: Project[];
  sessionsLoading?: boolean;
}) {
  const accounts = useRemote((signal) => listAccounts(c, signal), [c], {
    cache: { store: c.reads, key: ACCOUNT_LIST_KEY, ttl: ACCOUNT_INFO_TTL },
  });
  const [adding, setAdding] = useState(false);
  const content = useRef<HTMLDivElement>(null);
  const add = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!selected) return;
    const card = Array.from(
      content.current?.querySelectorAll<HTMLElement>("[data-account]") ?? [],
    ).find(
      (node) =>
        node.dataset.account ===
        accounts.data?.find((a) => matchesResource(a, selected))?.alias,
    );
    card?.scrollIntoView({ block: "start" });
  }, [selected, accounts.data]);
  return (
    <>
      <main className={`${resourceStyles.page} ${styles.page}`}>
        <PageToolbar className={styles.toolbar} aria-label="Account toolbar">
          <div className={styles.toolbarContent}>
            <div className={styles.toolbarActions}>
              <ToolbarIconButton
                ref={add}
                icon="add"
                label="Add account"
                onClick={() => setAdding(true)}
              />
            </div>
          </div>
        </PageToolbar>
        <div className={resourceStyles.body}>
          <SectionLayout ref={content} className={styles.layout}>
            {accounts.loading && !accounts.data && (
              <p className={resourceStyles.muted}>Loading accounts…</p>
            )}
            {accounts.error && <ErrorNotice error={accounts.error} />}
            {!accounts.loading && !accounts.data?.length && (
              <p className={resourceStyles.muted}>No accounts yet.</p>
            )}
            {accounts.data?.map((account) => (
              <article
                key={account.id.length ? key(account.id) : account.alias}
                data-account={account.alias}
                aria-label={account.name || account.alias}
                className={styles.card}
              >
                <AccountDetails
                  c={c}
                  account={account}
                  projects={projects}
                  sessions={sessionsForAccount(account, sessions)}
                  sessionsLoading={sessionsLoading}
                  refreshMetadata={accounts.reload}
                  metadataLoading={accounts.loading}
                  saved={(updated) => {
                    accounts.reload();
                    if (
                      selected === account.alias &&
                      updated.alias !== account.alias
                    )
                      select?.(updated.alias);
                  }}
                  erased={() => {
                    accounts.reload();
                    if (selected && matchesResource(account, selected))
                      select?.("");
                  }}
                />
              </article>
            ))}
          </SectionLayout>
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
