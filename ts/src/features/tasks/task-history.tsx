import { useEffect, useRef, useState } from "react";
import {
  Button,
  ScrollArea,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@lesomnus/nox";
import { Code, ConnectError } from "@connectrpc/connect";
import { key } from "@lesomnus/payday/store";
import type { Audit } from "#gen/cxz/payday/audit_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { projectId } from "#src/shared/api/project-id.ts";
import { taskRef } from "./task-model";
import styles from "./task-history.module.css";

const operations: Record<string, string> = {
  Add: "Created",
  Patch: "Updated",
  Apply: "Updated",
  Claim: "Assigned",
  Release: "Released",
};
const fieldNames: Record<number, string> = {
  4: "Alias",
  5: "Title",
  6: "Description",
  7: "Labels",
  8: "Project",
  9: "Status",
  10: "Priority",
  11: "Assigned to",
  12: "Assignee run",
};
export const taskHistoryAction = (row: Audit) =>
  operations[row.action.split("/").at(-1) ?? ""] ?? "Changed";
export const taskHistoryFields = (row: Audit) =>
  [
    ...new Set(row.fields.map((field) => fieldNames[field]).filter(Boolean)),
  ].join(", ");

export function TaskHistory({
  c,
  project,
  task,
  latest,
}: {
  c: Connection;
  project: Project;
  task: Task;
  latest?: Task;
}) {
  const [rows, setRows] = useState<Audit[]>([]);
  const [next, setNext] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController | undefined>(undefined);
  const revision = `${task.dateUpdated?.seconds}:${task.dateUpdated?.nanos}:${latest?.dateUpdated?.seconds}:${latest?.dateUpdated?.nanos}`;
  async function load(after = "") {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    try {
      const page = await c.tasks.history(
        { ref: taskRef(project, task), size: 20, after },
        { signal: controller.signal, timeoutMs: 15000 },
      );
      if (controller.signal.aborted) return;
      setRows((previous) =>
        after
          ? [
              ...new Map(
                [...previous, ...page.items].map((row) => [key(row.id), row]),
              ).values(),
            ]
          : page.items,
      );
      setNext(page.next);
    } catch (failure) {
      if (!controller.signal.aborted)
        setError(
          ConnectError.from(failure).code === Code.Unimplemented
            ? "Update the Manager and project runtime to view Task history."
            : String(failure),
        );
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }
  useEffect(() => {
    setRows([]);
    setNext("");
    void load();
    return () => request.current?.abort();
  }, [c, projectId(project), key(task.id), revision]);
  return (
    <div className={styles.history}>
      <h2>Change history</h2>
      <ScrollArea
        className={styles.scroll}
        aria-label="Task change history"
        scrollbarLabel="Scroll task history"
      >
        <Table
          className={styles.table}
          aria-label="Task history"
          aria-busy={loading}
        >
          <colgroup>
            <col className={styles.timeColumn} />
            <col />
          </colgroup>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Change</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={key(row.id)}>
                <TableCell className={styles.time}>
                  <DateTime value={row.dateCreated} showOffset={false} />
                </TableCell>
                <TableCell>
                  <span>{taskHistoryAction(row)}</span>
                  <small>{taskHistoryFields(row)}</small>
                </TableCell>
              </TableRow>
            ))}
            {!rows.length && (
              <TableRow>
                <TableCell colSpan={2} className={styles.empty}>
                  {loading
                    ? "Loading history…"
                    : error
                      ? "History unavailable"
                      : "No retained changes"}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </ScrollArea>
      {error && <ErrorNotice error={error} />}
      {(next || error) && (
        <Button
          type="button"
          loading={loading}
          disabled={loading}
          onClick={() => void load(error ? "" : next)}
        >
          {error ? "Retry history" : "Load more changes"}
        </Button>
      )}
    </div>
  );
}
