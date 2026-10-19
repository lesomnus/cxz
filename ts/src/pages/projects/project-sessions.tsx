import { useMemo, useState } from "react";
import { key } from "@lesomnus/payday/store";
import { PanelItem } from "@lesomnus/nox";
import { Link } from "@tanstack/react-router";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import { SessionIdentity } from "#src/features/session/components/session-identity.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./project-sessions.module.css";

export function ProjectSessions({
  sessions,
  accounts,
  loading,
}: {
  sessions: Session[];
  accounts: Account[];
  loading?: boolean;
}) {
  const [selected, select] = useState<Set<string>>(() => new Set());
  const accountsById = useMemo(
    () => new Map(accounts.map((a) => [key(a.id), a])),
    [accounts],
  );
  const count = sessions.filter((s) => selected.has(s.runtimeId)).length;
  const all = sessions.length > 0 && count === sessions.length;
  function toggle(id: string) {
    select((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  return (
    <PanelScroll
      className={styles.scroll}
      role="region"
      aria-label={t("Session list")}
      scrollbarLabel={t("Session list")}
    >
      <table
        className={styles.table}
        aria-label={t("Sessions")}
        aria-busy={loading}
      >
        <colgroup>
          <col className={styles.selectionColumn} />
          <col className={styles.sessionColumn} />
          <col className={styles.accountColumn} />
          <col />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">
              <input
                className={styles.checkbox}
                type="checkbox"
                aria-label={t("Select all sessions")}
                checked={all}
                disabled={!sessions.length}
                ref={(node) => {
                  if (node) node.indeterminate = count > 0 && !all;
                }}
                onChange={() =>
                  select(
                    all ? new Set() : new Set(sessions.map((s) => s.runtimeId)),
                  )
                }
              />
            </th>
            <th scope="col">{t("Session")}</th>
            <th scope="col">{t("Account")}</th>
            <th scope="col">{t("State")}</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => {
            const name = s.name || s.alias || s.runtimeId;
            const account = s.account && accountsById.get(key(s.account.id));
            const accountAlias = account?.alias || s.account?.alias;
            const accountName =
              account?.name ||
              s.account?.name ||
              account?.alias ||
              s.account?.alias ||
              "—";
            const checked = selected.has(s.runtimeId);
            return (
              <tr key={s.runtimeId} data-selected={checked || undefined}>
                <td>
                  <input
                    className={styles.checkbox}
                    type="checkbox"
                    aria-label={t("Select {name}", { name })}
                    checked={checked}
                    onChange={() => toggle(s.runtimeId)}
                  />
                </td>
                <td className={styles.cardCell}>
                  <PanelItem
                    type="button"
                    className={styles.session}
                    pressTarget=".session-heading"
                    aria-pressed={checked}
                    onClick={() => toggle(s.runtimeId)}
                  >
                    <SessionIdentity session={s} />
                    <OpenArrow />
                  </PanelItem>
                </td>
                <td className={`${styles.cardCell} ${styles.account}`}>
                  {accountAlias ? (
                    <Link
                      to="/accounts"
                      search={{ account: accountAlias }}
                      className={styles.accountCard}
                      title={accountName}
                    >
                      <span>{accountName}</span>
                      <OpenArrow />
                    </Link>
                  ) : (
                    accountName
                  )}
                </td>
                <td className={styles.state}>{stateLabel(s)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {!sessions.length && (
        <p className={styles.empty}>
          {loading ? t("Loading sessions…") : t("No sessions")}
        </p>
      )}
    </PanelScroll>
  );
}

function stateLabel(s: Session) {
  switch (s.status?.state) {
    case "running":
    case "working":
      return t("Working");
    case "waiting":
    case "waiting_input":
      return t("Waiting for input");
    case "idle":
      return t("Idle");
    case "stopped":
      return t("Stopped");
    case "interrupted":
      return t("Interrupted");
    case "failed":
      return t("Failed");
    default:
      return t("Unknown");
  }
}

function OpenArrow() {
  return (
    <svg
      className={styles.openArrow}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M7 17 17 7M7 7h10v10" />
    </svg>
  );
}
