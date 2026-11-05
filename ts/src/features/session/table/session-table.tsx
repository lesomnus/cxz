import { useMemo, useState } from "react";
import { key } from "@lesomnus/payday/store";
import {
  Button,
  EditableValue,
  TableRow,
  useRangeSelection,
} from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { SessionIndicator } from "#src/features/session/components/session-indicator.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  projectLocation,
  accountLocation,
  resourceHandle,
  sessionLocation,
} from "#src/shared/navigation/resource-route.ts";
import type { SessionControlConnection } from "#src/features/session/model/session-control-batch.ts";
import {
  editSession,
  validateSessionField,
} from "#src/features/session/model/session-edit.ts";
import { useSessionControls } from "./use-session-controls";
import {
  SessionTableMenu,
  SessionTableConfirmation,
  SessionTableFeedback,
} from "./session-table-controls";
import controlStyles from "./session-table-controls.module.css";
import styles from "./session-table.module.css";

export function SessionTable({
  c,
  sessions,
  accounts,
  projects = [],
  loading,
  addSession,
  showAccount = true,
  showProject = false,
  showToolbar = true,
  title = t("Sessions"),
  horizontalScrollbar = false,
  fillHeight = false,
  variant = "default",
}: {
  c: SessionControlConnection;
  sessions: Session[];
  accounts: Account[];
  projects?: Project[];
  loading?: boolean;
  addSession?: () => void;
  showAccount?: boolean;
  showProject?: boolean;
  showToolbar?: boolean;
  title?: string;
  horizontalScrollbar?: boolean;
  fillHeight?: boolean;
  variant?: "default" | "compact";
}) {
  const compact = variant === "compact";
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
  const projectsById = useMemo(
    () => new Map(projects.map((project) => [key(project.id), project])),
    [projects],
  );
  const selection = useRangeSelection({
    items: sessions.map((session) => session.runtimeId),
    selected,
    onSelectionChange: select,
    disabled: !!(controls.busy || loading),
  });
  const addControl = addSession && (
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
  );

  return (
    <>
      {showToolbar && (
        <div className={controlStyles.toolbar}>
          <h2>{title}</h2>
          {controls.busy && (
            <span className={controlStyles.progress} role="status">
              {t("Working…")} {controls.progress.done}/{controls.progress.total}
            </span>
          )}
          <SessionTableMenu
            sessions={chosen}
            controls={controls}
            bulk
            disabled={loading}
          />
          {addControl}
          <SessionTableFeedback controls={controls} />
        </div>
      )}
      <PanelScroll
        wheelBehavior="instant"
        scrollbarOffset={0}
        horizontalScrollbar={horizontalScrollbar}
        className={`${styles.scroll} ${compact ? styles.compact : ""} ${fillHeight ? styles.fillHeight : ""}`}
        role="region"
        aria-label={t("Session list")}
        scrollbarLabel={t("Session list")}
      >
        <table
          {...selection.getContainerProps()}
          className={styles.table}
          aria-label={t("Sessions")}
          aria-busy={loading}
          data-account-column={showAccount}
          data-project-column={showProject}
        >
          <colgroup>
            <col className={styles.selectionColumn} />
            {!compact && <col className={styles.agentColumn} />}
            <col className={styles.nameColumn} />
            {!compact && <col className={styles.aliasColumn} />}
            {showAccount && <col className={styles.accountColumn} />}
            {showProject && <col className={styles.projectColumn} />}
            <col className={styles.stateColumn} />
            <col className={styles.actionsColumn} />
          </colgroup>
          <thead>
            <tr>
              <th scope="col" className={styles.selectionCell}>
                <Checkbox
                  className={styles.cellCheckbox}
                  aria-label={t("Select all sessions")}
                  {...selection.getSelectAllProps()}
                />
              </th>
              {!compact && <th scope="col" aria-label={t("Agent")} />}
              <th scope="col">{t("Name")}</th>
              {!compact && <th scope="col">{t("Alias")}</th>}
              {showAccount && <th scope="col">{t("Account")}</th>}
              {showProject && <th scope="col">{t("Project")}</th>}
              <th scope="col" aria-label={compact ? t("State") : undefined}>
                {!compact && t("State")}
              </th>
              <th
                scope="col"
                className={styles.actions}
                aria-label={t("Controls")}
              >
                {!showToolbar && (
                  <SessionTableMenu
                    sessions={chosen}
                    controls={controls}
                    bulk
                    iconTrigger
                    disabled={loading}
                    addSession={addSession}
                  />
                )}
              </th>
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
              const projectResource =
                s.project && (projectsById.get(key(s.project.id)) || s.project);
              const project =
                projectResource && resourceHandle(projectResource);
              const projectName =
                projectResource?.name || projectResource?.alias || project;
              return (
                <TableRow key={s.runtimeId} selected={checked}>
                  <td className={styles.selectionCell}>
                    <Checkbox
                      className={styles.cellCheckbox}
                      aria-label={t("Select {name}", { name })}
                      {...selection.getCheckboxProps(s.runtimeId)}
                    />
                  </td>
                  {!compact && (
                    <td className={styles.agent}>
                      <AgentBrand agent={s.agent} />
                    </td>
                  )}
                  {compact ? (
                    <td className={`${styles.name} ${styles.cardCell}`}>
                      <RouteLink
                        rowAware
                        {...sessionLocation(s)}
                        className={styles.navigationButton}
                        title={name}
                      >
                        <span className={styles.identity}>
                          <span className={styles.title}>{name}</span>
                          <span className={styles.subtitle}>{s.alias}</span>
                        </span>
                        <OpenArrow />
                      </RouteLink>
                    </td>
                  ) : (
                    (["name", "alias"] as const).map((field) => (
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
                    ))
                  )}
                  {showAccount && (
                    <td className={styles.cardCell}>
                      {accountAlias ? (
                        <RouteLink
                          rowAware
                          {...accountLocation(accountAlias)}
                          className={styles.navigationButton}
                          title={accountName}
                        >
                          <span
                            className={styles.accountAgent}
                            aria-hidden="true"
                          >
                            <AgentBrand
                              agent={
                                account?.agent || s.account?.agent || s.agent
                              }
                            />
                          </span>
                          <span className={styles.identity}>
                            <span className={styles.title}>{accountName}</span>
                            <span className={styles.subtitle}>
                              {accountAlias}
                            </span>
                          </span>
                          <OpenArrow />
                        </RouteLink>
                      ) : (
                        accountName
                      )}
                    </td>
                  )}
                  {showProject && (
                    <td className={styles.cardCell}>
                      {project ? (
                        <RouteLink
                          rowAware
                          {...projectLocation(project)}
                          className={styles.navigationButton}
                          title={projectName}
                        >
                          <span className={styles.identity}>
                            <span className={styles.title}>{projectName}</span>
                            <span className={styles.subtitle}>
                              {projectResource?.alias}
                            </span>
                          </span>
                          <OpenArrow />
                        </RouteLink>
                      ) : (
                        "—"
                      )}
                    </td>
                  )}
                  <td className={styles.state}>
                    {compact ? (
                      <span className={styles.stateIndicator}>
                        <SessionIndicator session={s} variant="status" />
                      </span>
                    ) : (
                      stateLabel(s)
                    )}
                  </td>
                  <td className={`${styles.cardCell} ${styles.actions}`}>
                    <SessionTableMenu
                      sessions={[s]}
                      controls={controls}
                      disabled={loading}
                    />
                  </td>
                </TableRow>
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
      {!showToolbar && (
        <>
          {controls.busy && (
            <span className={controlStyles.progress} role="status">
              {t("Working…")} {controls.progress.done}/{controls.progress.total}
            </span>
          )}
          <SessionTableFeedback controls={controls} inline />
        </>
      )}
      <SessionTableConfirmation controls={controls} />
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
