import { DateTime } from "#src/shared/components/date-time.tsx";
import { useEffect, useId, useRef, useState } from "react";
import { Button, DialogActions, DialogCard } from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { TaskStatus, type Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import { ResourceValue } from "#src/shared/components/resource-value.tsx";
import { AgentIdentity } from "#src/shared/components/agent-identity.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  taskOwner,
  taskRef,
  taskLookup,
  taskStatusName,
  taskStatuses,
  ownerIssue,
} from "./task-model";
import surface from "#src/shared/components/resource-surface.module.css";
import styles from "./tasks.module.css";

function TaskEditor({
  c,
  project,
  task,
  latest,
  sessions,
  saved,
  close,
  setBusy,
  reload,
}: {
  c: Connection;
  project: Project;
  task?: Task;
  latest?: Task;
  sessions: Session[];
  saved: (task: Task) => void;
  close: () => void;
  setBusy: (busy: boolean) => void;
  reload: () => void;
}) {
  const id = useId();
  const [name, setName] = useState(task?.name ?? "");
  const [alias, setAlias] = useState(task?.alias ?? "");
  const [desc, setDesc] = useState(task?.desc ?? "");
  const [priority, setPriority] = useState(task?.priority ?? 2);
  const [status, setStatus] = useState(task?.status ?? TaskStatus.TODO);
  const [busy, busyState] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [confirmRelease, setConfirmRelease] = useState(false);
  const pending = useRef(false),
    controller = useRef<AbortController | undefined>(undefined);
  const creation = useRef(crypto.randomUUID());
  useEffect(() => () => controller.current?.abort(), []);
  const owner = sessions.find(
    (session) => session.runtimeId === task?.assignee,
  );
  const source = sessions.find(
    (session) => session.runtimeId === task?.sourceSession,
  );
  const issue = task ? ownerIssue(task, sessions) : "";
  const changed =
    !!task &&
    !!latest &&
    (task.dateUpdated?.seconds !== latest.dateUpdated?.seconds ||
      task.dateUpdated?.nanos !== latest.dateUpdated?.nanos);
  const owned = !!task?.assignee;
  const executable =
    owned &&
    owner?.status?.runId === task?.assigneeRun &&
    ["idle", "working", "waiting_input"].includes(owner?.status?.state ?? "");
  const statusOptions = taskStatuses.filter((item) =>
    owned
      ? executable
        ? item.value !== TaskStatus.TODO
        : item.value === task?.status
      : item.value === TaskStatus.TODO,
  );

  async function mutate(release = false) {
    if (pending.current) return false;
    pending.current = true;
    busyState(true);
    setBusy(true);
    setError("");
    const request = new AbortController();
    controller.current = request;
    try {
      const options = { signal: request.signal, timeoutMs: 15000 };
      const value = task
        ? release
          ? await c.tasks.release(
              {
                ref: taskRef(project, task),
                dateUpdated: task.dateUpdated,
                actor: taskOwner(task),
              },
              options,
            )
          : await c.tasks.patch(
              {
                ref: taskRef(project, task),
                dateUpdated: task.dateUpdated,
                name: name.trim(),
                alias: alias.trim(),
                desc,
                priority,
                ...(status !== task.status
                  ? {
                      status,
                      ...(status !== TaskStatus.CANCELED && owned
                        ? { actor: taskOwner(task) }
                        : {}),
                    }
                  : {}),
              },
              options,
            )
        : await c.tasks.add(
            {
              project: ref(project.runtimeId),
              clientId: creation.current,
              name: name.trim(),
              alias: alias.trim(),
              desc,
              priority,
            },
            options,
          );
      if (!request.signal.aborted) {
        c.taskCatalog.forProject(project.runtimeId).apply(value);
        saved(value);
      }
      return true;
    } catch (failure) {
      if (!request.signal.aborted) {
        const stale =
          ConnectError.from(failure).code === Code.FailedPrecondition;
        setConflict(stale);
        setError(
          stale
            ? "This task changed or its owner is no longer active. Your edits are preserved. Reload the latest task before retrying."
            : String(failure),
        );
      }
      return false;
    } finally {
      pending.current = false;
      if (!request.signal.aborted) {
        busyState(false);
        setBusy(false);
      }
    }
  }
  return (
    <>
      <form
        id={id}
        onSubmit={(event) => {
          event.preventDefault();
          void mutate();
        }}
      >
        <DialogCard
          title={task ? "Task details" : "Add task"}
          close={busy ? undefined : close}
          closeLabel="Close"
          footer={
            <DialogActions>
              {owned && (
                <Button
                  type="button"
                  disabled={busy}
                  onClick={() => setConfirmRelease(true)}
                >
                  Release
                </Button>
              )}
              <Button
                type="submit"
                form={id}
                disabled={busy || !name.trim() || conflict}
              >
                {busy ? "Saving…" : task ? "Save" : "Add"}
              </Button>
              <Button type="button" disabled={busy} onClick={close}>
                Cancel
              </Button>
            </DialogActions>
          }
        >
          <fieldset className={styles.form} disabled={busy}>
            <label className={surface.field}>
              Title
              <input
                autoFocus
                required
                maxLength={256}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label className={surface.field}>
              Alias
              <input
                maxLength={63}
                value={alias}
                placeholder={task ? undefined : "Generated automatically"}
                onChange={(event) => setAlias(event.target.value)}
              />
            </label>
            <label className={surface.field}>
              Description
              <textarea
                rows={5}
                maxLength={65536}
                value={desc}
                onChange={(event) => setDesc(event.target.value)}
              />
            </label>
            <ResourceValue
              label="Priority"
              value={String(priority)}
              options={["Highest", "High", "Normal", "Low", "Lowest"].map(
                (label, value) => ({ value: String(value), label }),
              )}
              choose={(value) => setPriority(Number(value))}
              disabled={busy}
            />
            {task && (
              <ResourceValue
                label="Status"
                value={String(status)}
                options={[
                  ...statusOptions.map((item) => ({
                    value: String(item.value),
                    label: item.label,
                  })),
                  { value: String(TaskStatus.CANCELED), label: "Canceled" },
                ]}
                choose={(value) => setStatus(Number(value) as TaskStatus)}
                disabled={busy}
              />
            )}
          </fieldset>
          {task && (
            <div className={styles.details}>
              <div>
                <span>Assigned to</span>
                {owner ? (
                  <RouteLink
                    to="/sessions/$sessionId"
                    params={{ sessionId: owner.runtimeId }}
                    className={styles.sessionLink}
                  >
                    <AgentIdentity
                      agent={owner.agent}
                      name={owner.name || owner.alias}
                      alias={owner.alias}
                    />
                  </RouteLink>
                ) : (
                  <span>
                    {task.assignee ? "Session unavailable" : "Unassigned"}
                  </span>
                )}
              </div>
              {issue && (
                <p className={styles.ownerIssue}>
                  {issue}. Release makes this task available to another session.
                </p>
              )}
              {source && (
                <div>
                  <span>Created from</span>
                  <RouteLink
                    to="/sessions/$sessionId"
                    params={{ sessionId: source.runtimeId }}
                    search={
                      task.sourceSeq ? { seq: String(task.sourceSeq) } : {}
                    }
                    className={styles.sessionLink}
                  >
                    {source.name || source.alias}
                  </RouteLink>
                </div>
              )}
              {!source && task.sourceSession && (
                <p className={styles.quiet}>
                  Created by a session that is no longer available.
                </p>
              )}
              {!!Object.keys(task.labels).length && (
                <dl className={styles.labels}>
                  {Object.entries(task.labels).map(([label, value]) => (
                    <div key={label}>
                      <dt>{label}</dt>
                      <dd>{value}</dd>
                    </div>
                  ))}
                </dl>
              )}
              <small className={styles.quiet}>
                {taskStatusName(task.status)} · Updated{" "}
                <DateTime value={task.dateUpdated} />
              </small>
            </div>
          )}
          {changed && !error && (
            <p role="status" className={styles.quiet}>
              This task was updated elsewhere. Reload to edit its latest
              version.
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {task && (changed || conflict) && (
            <Button type="button" disabled={busy} onClick={reload}>
              Reload latest task
            </Button>
          )}
        </DialogCard>
      </form>
      {confirmRelease && (
        <ConfirmationDialog
          title="Release task?"
          confirmLabel="Release"
          failureMessage={
            error ||
            "Could not release the task. Close this confirmation and reload its latest version."
          }
          close={() => setConfirmRelease(false)}
          execute={() => mutate(true)}
        >
          Return this task to Todo and clear its assignment? This does not stop
          the agent currently working on it.
        </ConfirmationDialog>
      )}
    </>
  );
}

export function TaskDialog({
  c,
  project,
  selected,
  latest,
  sessions,
  close,
  saved,
}: {
  c: Connection;
  project: Project;
  selected?: string;
  latest?: Task;
  sessions: Session[];
  close: () => void;
  saved: (task: Task) => void;
}) {
  const [busy, setBusy] = useState(false);
  const detail = useRemote(
    (signal) =>
      selected
        ? c.tasks.get(
            {
              ref: taskLookup(project, selected),
              select: { all: true },
            },
            { signal, timeoutMs: 10000 },
          )
        : Promise.resolve(undefined),
    [c, project.runtimeId, selected],
  );
  return (
    <AnchoredDialog
      title={selected ? "Task details" : "Add task"}
      centered
      close={close}
      busy={busy}
    >
      {!selected || detail.data ? (
        <TaskEditor
          key={
            detail.data
              ? `${detail.data.dateUpdated?.seconds}:${detail.data.dateUpdated?.nanos}`
              : "add"
          }
          c={c}
          project={project}
          task={detail.data}
          latest={latest}
          sessions={sessions}
          close={close}
          saved={saved}
          setBusy={setBusy}
          reload={detail.reload}
        />
      ) : (
        <DialogCard title="Task details" close={close} closeLabel="Close">
          {detail.error ? (
            <>
              <p role="alert">{detail.error}</p>
              <Button type="button" onClick={detail.reload}>
                Retry
              </Button>
            </>
          ) : (
            <p role="status">Loading task…</p>
          )}
        </DialogCard>
      )}
    </AnchoredDialog>
  );
}
