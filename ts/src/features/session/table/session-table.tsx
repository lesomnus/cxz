import { Icon } from "#src/shared/icons/icon.tsx";
import { EditableValue } from "#src/shared/components/editable-value.tsx";
import { useMemo, useState, type CSSProperties } from "react";
import { key } from "@lesomnus/payday/store";
import { Button, TableRow, useRangeSelection } from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { SessionIndicator } from "#src/features/session/components/session-indicator.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { ResourceNavigationButton } from "#src/shared/components/resource-navigation-button.tsx";
import { SessionNavigationButton } from "#src/shared/components/session-navigation-button.tsx";
import {
  projectLocation,
  accountLocation,
  resourceHandle,
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
import resourceTableStyles from "#src/shared/components/resource-table.module.css";

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
  height,
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
  /** Override the bounded dashboard height, for example auto in a page list. */
  height?: CSSProperties["height"];
  variant?: "default" | "compact" | "overview";
}) {
  const compact = variant !== "default";
  const overview = variant === "overview";
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
  const stateHeading = (
    <th scope="col" aria-label={compact ? t("State") : undefined}>
      {!compact && t("State")}
    </th>
  );
  const addControl = addSession && (
    <Button
      type="button"
      className={controlStyles.addTrigger}
      aria-label={t("Add session")}
      disabled={loading || controls.busy}
      onClick={addSession}
    >
      <Icon name="add" aria-hidden="true" />
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
        style={height === undefined ? undefined : { height }}
        wheelBehavior="instant"
        scrollbarOffset={0}
        horizontalScrollbar={horizontalScrollbar}
        className={`${styles.scroll} ${compact ? styles.compact : ""} ${overview ? `${styles.overview} ${resourceTableStyles.layout}` : ""} ${fillHeight ? styles.fillHeight : ""}`}
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
            {overview && <col className={styles.stateColumn} />}
            {!compact && <col className={styles.agentColumn} />}
            <col className={styles.nameColumn} />
            {!compact && <col className={styles.aliasColumn} />}
            {showAccount && <col className={styles.accountColumn} />}
            {showProject && <col className={styles.projectColumn} />}
            {!overview && <col className={styles.stateColumn} />}
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
              {overview && stateHeading}
              {!compact && <th scope="col" aria-label={t("Agent")} />}
              <th scope="col">{t("Name")}</th>
              {!compact && <th scope="col">{t("Alias")}</th>}
              {showAccount && <th scope="col">{t("Account")}</th>}
              {showProject && <th scope="col">{t("Project")}</th>}
              {!overview && stateHeading}
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
              const state = (
                <td className={styles.state}>
                  {compact ? (
                    <span className={styles.stateIndicator}>
                      <SessionIndicator session={s} variant="status" />
                    </span>
                  ) : (
                    stateLabel(s)
                  )}
                </td>
              );
              return (
                <TableRow key={s.runtimeId} selected={checked}>
                  <td className={styles.selectionCell}>
                    <Checkbox
                      className={styles.cellCheckbox}
                      aria-label={t("Select {name}", { name })}
                      {...selection.getCheckboxProps(s.runtimeId)}
                    />
                  </td>
                  {overview && state}
                  {!compact && (
                    <td className={styles.agent}>
                      <AgentBrand agent={s.agent} />
                    </td>
                  )}
                  {compact ? (
                    <td className={`${styles.name} ${styles.cardCell}`}>
                      <SessionNavigationButton session={s} />
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
                        <ResourceNavigationButton
                          {...accountLocation(accountAlias)}
                          name={accountName}
                          alias={accountAlias}
                          leading={
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
                          }
                        />
                      ) : (
                        accountName
                      )}
                    </td>
                  )}
                  {showProject && (
                    <td className={styles.cardCell}>
                      {project ? (
                        <ResourceNavigationButton
                          {...projectLocation(project)}
                          name={projectName || project}
                          alias={projectResource?.alias}
                        />
                      ) : (
                        "—"
                      )}
                    </td>
                  )}
                  {!overview && state}
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
