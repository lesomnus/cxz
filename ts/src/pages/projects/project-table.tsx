import { useMemo, useState } from "react";
import { key } from "@lesomnus/payday/store";
import {
  CompactNumber,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { containerState } from "./project-state";
import { ProjectStateIcon } from "./project-state-icon";
import { ProjectActions } from "./project-actions";
import styles from "./project-table.module.css";

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
  const selectedCount = projects.filter((project) =>
    selected.has(key(project.id)),
  ).length;
  const all = projects.length > 0 && selectedCount === projects.length;
  function toggle(id: string) {
    select((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  const counts = useMemo(() => {
    const values = new Map<string, number>();
    for (const session of sessions) {
      if (!session.project) continue;
      const id = key(session.project.id);
      values.set(id, (values.get(id) ?? 0) + 1);
    }
    return values;
  }, [sessions]);
  return (
    <Table
      className={styles.table}
      aria-label={t("Projects")}
      aria-busy={loading || undefined}
    >
      <colgroup>
        <col className={styles.selectionColumn} />
        <col className={styles.nameColumn} />
        <col className={styles.countColumn} />
        <col className={styles.stateColumn} />
        <col className={styles.actionsColumn} />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead className={styles.selectionCell}>
            <Checkbox
              className={styles.cellCheckbox}
              aria-label={t("Select all projects")}
              checked={all}
              indeterminate={selectedCount > 0 && !all}
              disabled={loading || !projects.length}
              onChange={() =>
                select(
                  all
                    ? new Set()
                    : new Set(projects.map((project) => key(project.id))),
                )
              }
            />
          </TableHead>
          <TableHead>{t("Name")}</TableHead>
          <TableHead align="right">{t("Sessions")}</TableHead>
          <TableHead aria-label={t("State")} />
          <TableHead aria-label={t("Actions")} />
        </TableRow>
      </TableHeader>
      <TableBody>
        {projects.map((project) => (
          <TableRow
            key={key(project.id)}
            selected={selected.has(key(project.id))}
          >
            <TableCell className={styles.selectionCell}>
              <Checkbox
                className={styles.cellCheckbox}
                aria-label={t("Select {name}", {
                  name:
                    project.name || project.alias || resourceHandle(project),
                })}
                checked={selected.has(key(project.id))}
                onChange={() => toggle(key(project.id))}
              />
            </TableCell>
            <TableCell className={styles.nameCell}>
              <RouteLink
                {...projectLocation(resourceHandle(project))}
                className={styles.projectLink}
              >
                <span className={styles.identity}>
                  <span className={styles.name}>
                    {project.name || project.alias || resourceHandle(project)}
                  </span>
                  <small className={styles.alias}>{project.alias}</small>
                </span>
              </RouteLink>
            </TableCell>
            <TableCell align="right">
              <CompactNumber
                value={
                  sessionsLoading ? NaN : (counts.get(key(project.id)) ?? 0)
                }
                mode="integer"
                digits={3}
                label={sessionsLoading ? t("Loading sessions…") : undefined}
              />
            </TableCell>
            <TableCell className={styles.state}>
              <ProjectStateIcon state={containerState(project)} />
            </TableCell>
            <TableCell className={styles.actionsCell}>
              <ProjectActions
                c={c}
                project={project}
                sessions={sessions}
                updated={updated}
                erased={() =>
                  select((previous) => {
                    const next = new Set(previous);
                    next.delete(key(project.id));
                    return next;
                  })
                }
                variant="menu"
              />
            </TableCell>
          </TableRow>
        ))}
        {!projects.length && (
          <TableRow>
            <TableCell colSpan={5} className={styles.empty}>
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
