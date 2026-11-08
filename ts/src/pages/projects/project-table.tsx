import { Icon } from "#src/shared/icons/icon.tsx";
import { useMemo, useState, useSyncExternalStore } from "react";
import { key } from "@lesomnus/payday/store";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  useRangeSelection,
} from "@lesomnus/nox";
import { ContainerState, type Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { containerState } from "./project-state";
import { ProjectStateIcon } from "./project-state-icon";
import { ProjectActions } from "./project-actions";
import { projectActivitySessions } from "./project-session-activity";
import { useSessionActivity } from "./use-session-activity";
import styles from "./project-table.module.css";
import resourceTableStyles from "#src/shared/components/resource-table.module.css";
import { SessionNavigationButton } from "#src/shared/components/session-navigation-button.tsx";

export function ProjectTable({
  c,
  projects,
  sessions,
  loading,
  sessionsLoading,
  updated,
}: {
  c: Connection;
  projects: Project[];
  sessions: Session[];
  loading: boolean;
  sessionsLoading?: boolean;
  updated: (project: Project) => void;
}) {
  const [selected, select] = useState<Set<string>>(() => new Set());
  const chosen = projects.filter((project) => selected.has(key(project.id)));
  const selection = useRangeSelection({
    items: projects.map((project) => key(project.id)),
    selected,
    onSelectionChange: select,
    disabled: loading,
  });
  const completed = useSyncExternalStore(
    c.attention.subscribe,
    c.attention.completedSnapshot,
  );
  const activity = useSessionActivity(c, sessions);
  const activeSessions = useMemo(
    () => projectActivitySessions(sessions, completed, activity),
    [sessions, completed, activity],
  );
  function unselect(projects: Project[]) {
    select((previous) => {
      const next = new Set(previous);
      for (const project of projects) next.delete(key(project.id));
      return next;
    });
  }
  return (
    <Table
      {...selection.getContainerProps()}
      className={`${styles.table} ${resourceTableStyles.table} ${resourceTableStyles.layout}`}
      aria-label={t("Projects")}
      aria-busy={loading || undefined}
    >
      <colgroup>
        <col className={styles.selectionColumn} />
        <col className={styles.stateColumn} />
        <col className={styles.nameColumn} />
        <col className={styles.nameColumn} />
        <col className={styles.activityColumn} />
        <col />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead className={styles.selectionCell}>
            <Checkbox
              className={styles.cellCheckbox}
              aria-label={t("Select all projects")}
              {...selection.getSelectAllProps()}
            />
          </TableHead>
          <TableHead aria-label={t("State")} />
          <TableHead>{t("Name")}</TableHead>
          <TableHead>{t("Recent session")}</TableHead>
          <TableHead aria-label={t("Last activity")} />
          <TableHead className={styles.actionsCell} aria-label={t("Actions")}>
            <div className={styles.actionSlot}>
              <ProjectActions
                c={c}
                projects={chosen}
                sessions={sessions}
                updated={updated}
                erased={(p) => unselect([p])}
                completed={unselect}
                variant="menu"
              />
            </div>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {projects.map((project) => {
          const active = activeSessions.get(key(project.id));
          const state = containerState(project);
          const inactive =
            state === ContainerState.ABSENT || state === ContainerState.STOPPED;
          const time = active && activity.get(active.runtimeId)?.time;
          return (
            <TableRow
              key={key(project.id)}
              className={resourceTableStyles.row}
              selected={selected.has(key(project.id))}
              data-inactive={inactive || undefined}
            >
              <TableCell className={styles.selectionCell}>
                <Checkbox
                  className={styles.cellCheckbox}
                  aria-label={t("Select {name}", {
                    name:
                      project.name || project.alias || resourceHandle(project),
                  })}
                  {...selection.getCheckboxProps(key(project.id))}
                />
              </TableCell>
              <TableCell className={styles.state}>
                <ProjectStateIcon state={state} />
              </TableCell>
              <TableCell className={styles.nameCell}>
                <RouteLink
                  rowAware
                  {...projectLocation(resourceHandle(project))}
                  className={styles.projectLink}
                >
                  <span className={styles.identity}>
                    <span className={styles.name}>
                      {project.name || project.alias || resourceHandle(project)}
                    </span>
                    <small className={styles.alias}>{project.alias}</small>
                  </span>
                  <NavigationArrow />
                </RouteLink>
              </TableCell>
              <TableCell className={styles.nameCell}>
                {active ? (
                  <SessionNavigationButton session={active} />
                ) : (
                  <span className={styles.muted}>
                    {sessionsLoading ? t("Loading sessions…") : "—"}
                  </span>
                )}
              </TableCell>
              <TableCell className={styles.activity}>
                {time ? (
                  <RelativeTime value={time} />
                ) : (
                  <span className={styles.muted}>—</span>
                )}
              </TableCell>
              <TableCell className={styles.actionsCell}>
                <div className={styles.actionSlot}>
                  <ProjectActions
                    c={c}
                    project={project}
                    sessions={sessions}
                    updated={updated}
                    erased={(p) => unselect([p])}
                    variant="menu"
                  />
                </div>
              </TableCell>
            </TableRow>
          );
        })}
        {!projects.length && (
          <TableRow>
            <TableCell colSpan={6} className={styles.empty}>
              <span role="status">
                {loading ? t("Loading projects…") : t("No projects yet.")}
              </span>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}

function NavigationArrow({ diagonal = false }: { diagonal?: boolean }) {
  return (
    <Icon
      name={diagonal ? "open-external" : "open-project"}
      className={styles.arrow}
      aria-hidden="true"
    />
  );
}
