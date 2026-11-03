import { useMemo } from "react";
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
import { t } from "#src/shared/i18n/i18n.ts";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { containerState, containerStateName } from "./project-state";
import styles from "./project-table.module.css";

export function ProjectTable({
  projects,
  sessions,
  loading,
  sessionsLoading,
}: {
  projects: Project[];
  sessions: Session[];
  loading: boolean;
  sessionsLoading?: boolean;
}) {
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
        <col className={styles.nameColumn} />
        <col />
        <col className={styles.countColumn} />
        <col className={styles.stateColumn} />
        <col className={styles.updatedColumn} />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead>{t("Name")}</TableHead>
          <TableHead>{t("Workspace")}</TableHead>
          <TableHead align="right">{t("Sessions")}</TableHead>
          <TableHead>{t("State")}</TableHead>
          <TableHead>{t("Updated")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {projects.map((project) => (
          <TableRow key={key(project.id)}>
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
            <TableCell>
              <span className={styles.path} title={project.workspace}>
                {project.workspace}
              </span>
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
              {t(containerStateName(containerState(project)))}
            </TableCell>
            <TableCell>
              <div className={styles.updated}>
                <DateTime value={project.dateUpdated} />
                <small>
                  <RelativeTime value={project.dateUpdated} />
                </small>
              </div>
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
