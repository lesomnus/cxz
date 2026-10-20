import { useMemo, useState } from "react";
import { key } from "@lesomnus/payday/store";
import { Link } from "@tanstack/react-router";
import { Button, EditableValue } from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import type { SessionControlConnection } from "#src/features/session/model/session-control-batch.ts";
import {
  editSession,
  validateSessionField,
} from "#src/features/session/model/session-edit.ts";
import { useSessionControls } from "./use-session-controls";
import {
  ProjectSessionMenu,
  ProjectSessionConfirmation,
  ProjectSessionFeedback,
} from "./project-session-controls";
import controlStyles from "./project-session-controls.module.css";
import styles from "./project-sessions.module.css";

export function ProjectSessions({
  c,
  sessions,
  accounts,
  loading,
  addSession,
}: {
  c: SessionControlConnection;
  sessions: Session[];
  accounts: Account[];
  loading?: boolean;
  addSession: () => void;
}) {
  const [selected, select] = useState<Set<string>>(() => new Set());
  const controls = useSessionControls(c, (ids) =>
    select((previous) => {
      const next = new Set(previous);
      for (const id of ids) next.delete(id);
      return next;
    }),
  );
  const chosen = sessions.filter((s) => selected.has(s.runtimeId));

  const accountsById = useMemo(
    () => new Map(accounts.map((a) => [key(a.id), a])),
    [accounts],
  );
  const count = chosen.length;
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
    <>
      <div className={controlStyles.toolbar}>
        <h2>{t("Sessions")}</h2>
        {controls.busy && (
          <span className={controlStyles.progress} role="status">
            {t("Working…")} {controls.progress.done}/{controls.progress.total}
          </span>
        )}
        <ProjectSessionMenu
          sessions={chosen}
          controls={controls}
          bulk
          disabled={loading}
        />
        <Button
          type="button"
          className={controlStyles.addTrigger}
          aria-label={t("Add session")}
          disabled={loading || controls.busy}
          onClick={addSession}
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
          <span>{t("Session")}</span>
        </Button>
        <ProjectSessionFeedback controls={controls} />
      </div>
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
            <col className={styles.agentColumn} />
            <col className={styles.nameColumn} />
            <col className={styles.aliasColumn} />
            <col className={styles.accountColumn} />
            <col />
            <col className={styles.actionsColumn} />
          </colgroup>
          <thead>
            <tr>
              <th scope="col" className={styles.selectionCell}>
                <Checkbox
                  className={styles.cellCheckbox}
                  aria-label={t("Select all sessions")}
                  checked={all}
                  disabled={!sessions.length || controls.busy || loading}
                  indeterminate={count > 0 && !all}
                  onChange={() =>
                    select(
                      all
                        ? new Set()
                        : new Set(sessions.map((s) => s.runtimeId)),
                    )
                  }
                />
              </th>
              <th scope="col" aria-label={t("Agent")} />
              <th scope="col">{t("Name")}</th>
              <th scope="col">{t("Alias")}</th>
              <th scope="col">{t("Account")}</th>
              <th scope="col">{t("State")}</th>
              <th scope="col" aria-label={t("Controls")} />
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
                  <td className={styles.selectionCell}>
                    <Checkbox
                      className={styles.cellCheckbox}
                      aria-label={t("Select {name}", { name })}
                      checked={checked}
                      disabled={controls.busy || loading}
                      onChange={() => toggle(s.runtimeId)}
                    />
                  </td>
                  <td className={styles.agent}>
                    <AgentBrand agent={s.agent} />
                  </td>
                  {(["name", "alias"] as const).map((field) => (
                    <td className={styles[field]} key={field}>
                      <fieldset
                        className={styles.editable}
                        disabled={controls.busy || loading}
                      >
                        <EditableValue
                          label={field === "name" ? t("Name") : t("Alias")}
                          value={s[field]}
                          monospace={field === "alias"}
                          validate={(value) =>
                            validateSessionField(field, value)
                          }
                          save={async (value) => {
                            const updated = await editSession(
                              c.sessions,
                              s.runtimeId,
                              field,
                              value,
                            );
                            c.store.apply("cxz.Session", [
                              { id: updated.id, value: updated },
                            ]);
                          }}
                        />
                      </fieldset>
                    </td>
                  ))}
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
                  <td className={`${styles.cardCell} ${styles.actions}`}>
                    <ProjectSessionMenu
                      sessions={[s]}
                      controls={controls}
                      disabled={loading}
                    />
                  </td>
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
      <ProjectSessionConfirmation controls={controls} />
    </>
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
