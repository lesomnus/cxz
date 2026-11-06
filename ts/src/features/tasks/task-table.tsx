import { Icon } from "#src/shared/icons/icon.tsx";
import { useEffect, useMemo, useState } from "react";
import {
  Button,
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import { TaskStatus } from "#gen/cxz/task_pb";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { TaskPriority, TaskStatusIcon, TaskTime } from "./task-indicators";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import {
  ownerIssue,
  tableTasks,
  taskTablePage,
  taskHistoryPageSize,
  taskKey,
  taskStatusName,
} from "./task-model";
import styles from "./task-table.module.css";
import resourceStyles from "#src/shared/components/resource-table.module.css";

export function TaskTable({
  tasks,
  sessions,
  sessionsLoading,
  assignee = "",
  run = "",
  open,
}: {
  tasks: Task[];
  sessions: Session[];
  sessionsLoading?: boolean;
  assignee?: string;
  run?: string;
  open: (task: Task) => void;
}) {
  const ordered = useMemo(
    () => tableTasks(tasks, assignee, run),
    [tasks, assignee, run],
  );
  const owners = useMemo(
    () => new Map(sessions.map((session) => [session.runtimeId, session])),
    [sessions],
  );
  const { locale } = useLocale();
  const [now, setNow] = useState(Date.now);
  const [olderDone, setOlderDone] = useState(0);
  const { rows, hidden } = useMemo(
    () => taskTablePage(ordered, now, olderDone),
    [ordered, now, olderDone],
  );
  useEffect(() => {
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") setNow(Date.now());
    }, 30_000);
    return () => clearInterval(timer);
  }, []);
  return (
    <Table
      className={`${styles.table} ${resourceStyles.layout}`}
      aria-label="Tasks"
    >
      <colgroup>
        <col className={styles.statusColumn} />
        <col />
        <col className={styles.ownerColumn} />
        <col className={styles.priorityColumn} />
        <col className={styles.dateColumn} />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead aria-label="Status" />
          <TableHead>Task</TableHead>
          <TableHead>Assignee</TableHead>
          <TableHead aria-label="Priority" />
          <TableHead>Updated</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((task) => {
          const owner = owners.get(task.assignee);
          const issue = ownerIssue(task, sessions, sessionsLoading);
          return (
            <TableRow
              key={taskKey(task)}
              data-task-alias={task.alias}
              data-task-status={taskStatusName(task.status)}
              data-done={task.status === TaskStatus.DONE || undefined}
            >
              <TableCell className={styles.status}>
                <TaskStatusIcon status={task.status} />
              </TableCell>
              <TableCell className={styles.buttonCell}>
                <Button
                  rowAware
                  className={styles.taskButton}
                  type="button"
                  onClick={() => open(task)}
                  aria-label={`Open task: ${task.name}`}
                  aria-haspopup="dialog"
                  title={task.name}
                >
                  <span className={styles.identity}>
                    <span className={styles.name}>{task.name}</span>
                    <span className={styles.alias}>{task.alias}</span>
                  </span>
                  <Icon
                    name="open-details"
                    className={styles.openIcon}
                    aria-hidden="true"
                  />
                </Button>
              </TableCell>
              <TableCell className={styles.buttonCell}>
                {owner ? (
                  <RouteLink
                    rowAware
                    className={styles.ownerButton}
                    {...sessionLocation(owner)}
                    title={owner.name || owner.alias}
                  >
                    <AgentBrand agent={owner.agent} />
                    <span className={styles.identity}>
                      <span className={styles.ownerAlias}>
                        {owner.alias || owner.runtimeId}
                      </span>
                      <span className={styles.alias}>
                        {issue || owner.name}
                      </span>
                    </span>
                    <Icon
                      name="open-external"
                      className={styles.openIcon}
                      aria-hidden="true"
                    />
                  </RouteLink>
                ) : (
                  <span className={styles.secondary}>
                    {task.assignee
                      ? sessionsLoading
                        ? "Loading sessions…"
                        : "Session unavailable"
                      : "Unassigned"}
                  </span>
                )}
              </TableCell>
              <TableCell className={styles.priorityCell}>
                <TaskPriority priority={task.priority} />
              </TableCell>
              <TableCell className={styles.time}>
                <TaskTime value={task.dateUpdated} now={now} locale={locale} />
              </TableCell>
            </TableRow>
          );
        })}
        {!rows.length && !hidden && (
          <TableRow>
            <TableCell colSpan={5} className={styles.empty}>
              <span role="status">No tasks</span>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
      {hidden > 0 && (
        <TableFooter>
          <TableRow>
            <TableCell colSpan={5} className={styles.history}>
              <Button
                type="button"
                onClick={() =>
                  setOlderDone((count) => count + taskHistoryPageSize)
                }
              >
                Load more
              </Button>
            </TableCell>
          </TableRow>
        </TableFooter>
      )}
    </Table>
  );
}
