import { useEffect, useMemo, useState } from "react";
import {
  Button,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  useRangeSelection,
} from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import { TaskStatus } from "#gen/cxz/task_pb";
import { Checkbox } from "#src/shared/components/checkbox.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { TaskPriority, TaskStatusIcon, TaskTime } from "./task-indicators";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { ownerIssue, tableTasks, taskKey, taskStatusName } from "./task-model";
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
  const rows = useMemo(
    () => tableTasks(tasks, assignee, run),
    [tasks, assignee, run],
  );
  const owners = useMemo(
    () => new Map(sessions.map((session) => [session.runtimeId, session])),
    [sessions],
  );
  const [selected, select] = useState<Set<string>>(() => new Set());
  const selection = useRangeSelection({
    items: rows.map(taskKey),
    selected,
    onSelectionChange: select,
  });
  const { locale } = useLocale();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") setNow(Date.now());
    }, 30_000);
    return () => clearInterval(timer);
  }, []);
  return (
    <Table
      {...selection.getContainerProps()}
      className={`${styles.table} ${resourceStyles.layout}`}
      aria-label="Tasks"
    >
      <colgroup>
        <col className={styles.selectionColumn} />
        <col className={styles.statusColumn} />
        <col />
        <col className={styles.ownerColumn} />
        <col className={styles.priorityColumn} />
        <col className={styles.dateColumn} />
        <col className={styles.dateColumn} />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead className={styles.selectionCell} aria-label="Selection">
            <Checkbox
              className={styles.cellCheckbox}
              aria-label="Select all tasks"
              {...selection.getSelectAllProps()}
            />
          </TableHead>
          <TableHead aria-label="Status" />
          <TableHead>Task</TableHead>
          <TableHead>Assignee</TableHead>
          <TableHead align="center">Priority</TableHead>
          <TableHead>Updated</TableHead>
          <TableHead>Created</TableHead>
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
              selected={selected.has(taskKey(task))}
            >
              <TableCell className={styles.selectionCell}>
                <Checkbox
                  className={styles.cellCheckbox}
                  aria-label={`Select ${task.name}`}
                  {...selection.getCheckboxProps(taskKey(task))}
                />
              </TableCell>
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
                  <svg
                    className={styles.openIcon}
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M9 4H4v5m11-5h5v5M4 15v5h5m11-5v5h-5" />
                  </svg>
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
              <TableCell className={styles.time}>
                <TaskTime value={task.dateCreated} now={now} locale={locale} />
              </TableCell>
            </TableRow>
          );
        })}
        {!rows.length && (
          <TableRow>
            <TableCell colSpan={7} className={styles.empty}>
              <span role="status">No tasks</span>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}
