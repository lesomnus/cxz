import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import { useEffect, useId, useRef, useState, type CSSProperties } from "react";
import {
  Button,
  CardActions,
  Card,
  Form,
  FormField,
  SegmentedControl,
  ValueMenu,
  Section,
  Popover,
  type ButtonProps,
} from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { Code, ConnectError } from "@connectrpc/connect";
import { Id } from "@lesomnus/payday/pdid";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AnchoredDialog } from "#src/shared/components/anchored-dialog.tsx";
import { useDialogClose } from "#src/shared/components/dialog-close.ts";
import {
  taskPriorities,
  taskOwner,
  taskRef,
  taskLookup,
  taskStatusName,
  ownerIssue,
} from "./task-model";
import {
  TaskPrioritySymbol,
  TaskStatusIcon,
  taskPriorityMask,
} from "./task-indicators";
import { TaskTitleField } from "./task-title-field";
import {
  TaskDescriptionField,
  defaultDescriptionOptions,
  type TaskDescriptionOptions,
} from "./task-description-field";
import detailStyles from "./task-detail-panels.module.css";
import styles from "./tasks.module.css";
import { TaskDetailPanels, TaskMetadata } from "./task-detail-panels";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import actionStyles from "#src/shared/components/resource-action.module.css";

function TaskActionButton({
  panel,
  label,
  icon,
  ...props
}: ButtonProps & { panel: boolean; label: string; icon: IconName }) {
  if (!panel) return <Button {...props}>{label}</Button>;
  return (
    <Popover content={label} placement="top">
      <Section
        {...props}
        as="button"
        aria-label={label}
        className={actionStyles.button}
      >
        <Icon name={icon} />
      </Section>
    </Popover>
  );
}

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
  reloadError,
  presentation = "dialog",
  onDirtyChange,
  descriptionOptions = defaultDescriptionOptions,
  onDescriptionOptionsChange = () => {},
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
  reloadError?: string;
  presentation?: "dialog" | "page";
  onDirtyChange?: (dirty: boolean) => void;
  descriptionOptions?: TaskDescriptionOptions;
  onDescriptionOptionsChange?: (value: TaskDescriptionOptions) => void;
}) {
  const id = useId();
  const [name, setName] = useState(task?.name ?? "");
  const [alias, setAlias] = useState(task?.alias ?? "");
  const [desc, setDesc] = useState(task?.desc ?? "");
  const [priority, setPriority] = useState(task?.priority ?? 2);
  const dirty =
    name !== (task?.name ?? "") ||
    alias !== (task?.alias ?? "") ||
    desc !== (task?.desc ?? "") ||
    priority !== (task?.priority ?? 2);
  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);
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
    ((latest.dateUpdated?.seconds ?? 0n) > (task.dateUpdated?.seconds ?? 0n) ||
      ((latest.dateUpdated?.seconds ?? 0n) ===
        (task.dateUpdated?.seconds ?? 0n) &&
        (latest.dateUpdated?.nanos ?? 0) > (task.dateUpdated?.nanos ?? 0)));
  const owned = !!task?.assignee;
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
              },
              options,
            )
        : await c.tasks.add(
            {
              project: projectRef(projectId(project)),
              clientId: creation.current,
              name: name.trim(),
              alias: alias.trim(),
              desc,
              priority,
            },
            options,
          );
      if (!request.signal.aborted) {
        c.taskCatalog.forProject(projectId(project)).apply(value);
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
  const priorityField = (
    <FormField label="Priority" fullWidth={!task}>
      {task ? (
        <ValueMenu
          label="Priority"
          alignItemWithTrigger
          value={String(priority)}
          options={taskPriorities.map((label, value) => ({
            value: String(value),
            label,
            icon: <TaskPrioritySymbol priority={value} />,
          }))}
          choose={(value) => setPriority(Number(value))}
          disabled={busy}
        />
      ) : (
        <div className={styles.priorityControl}>
          <SegmentedControl
            label="Priority"
            value={String(priority)}
            options={taskPriorities.map((label, value) => ({
              value: String(value),
              label,
              icon: (
                <span className={styles.prioritySymbol}>
                  <TaskPrioritySymbol priority={value} />
                </span>
              ),
            }))}
            onChange={(value) => setPriority(Number(value))}
            disabled={busy}
          />
        </div>
      )}
    </FormField>
  );
  const statusField = task && (
    <FormField label="Status">
      <div className={detailStyles.readonlyStatus} aria-label="Status">
        <TaskStatusIcon status={task.status} />
        <span>{taskStatusName(task.status)}</span>
      </div>
    </FormField>
  );
  const taskIdField = task && (
    <div className={detailStyles.taskId}>
      <ResourceFields
        columns={1}
        rows={[
          {
            label: "Task ID",
            value:
              task.id.length === 16 ? new Id(task.id).toString() : undefined,
            identifier: true,
          },
        ]}
      />
    </div>
  );
  const titleField =
    presentation === "page" ? (
      <TaskTitleField
        id={`${id}-title`}
        value={name}
        disabled={busy}
        change={setName}
      />
    ) : (
      <FormField label="Title" htmlFor={`${id}-title`} fullWidth>
        <input
          id={`${id}-title`}
          autoFocus={presentation === "dialog"}
          required
          disabled={busy}
          maxLength={256}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </FormField>
    );
  const aliasField = (
    <FormField label="Alias" htmlFor={`${id}-alias`} fullWidth>
      <input
        id={`${id}-alias`}
        disabled={busy}
        maxLength={63}
        value={alias}
        placeholder={task ? undefined : "Generated automatically"}
        onChange={(event) => setAlias(event.target.value)}
      />
    </FormField>
  );
  const descriptionField =
    presentation === "page" ? (
      <TaskDescriptionField
        id={`${id}-description`}
        value={desc}
        disabled={busy}
        change={setDesc}
        options={descriptionOptions}
        setOptions={onDescriptionOptionsChange}
      />
    ) : (
      <FormField label="Description" htmlFor={`${id}-description`} fullWidth>
        <textarea
          id={`${id}-description`}
          className={styles.description}
          rows={5}
          disabled={busy}
          maxLength={65536}
          value={desc}
          onChange={(event) => setDesc(event.target.value)}
        />
      </FormField>
    );
  const actions = (
    <>
      {owned && (
        <TaskActionButton
          panel={presentation === "page"}
          label="Release"
          icon="sign-out"
          type="button"
          disabled={busy}
          onClick={() => setConfirmRelease(true)}
        />
      )}
      <TaskActionButton
        panel={presentation === "page"}
        label={task ? "Save" : "Confirm"}
        icon="save"
        type="submit"
        form={id}
        loading={busy}
        disabled={
          busy ||
          !name.trim() ||
          conflict ||
          (presentation === "page" && !dirty)
        }
      />
      <TaskActionButton
        panel={presentation === "page"}
        label={presentation === "dialog" ? "Cancel" : "Discard changes"}
        icon="undo"
        type="button"
        disabled={busy || (presentation === "page" && !dirty)}
        onClick={
          presentation === "dialog"
            ? close
            : () => {
                setName(task?.name ?? "");
                setAlias(task?.alias ?? "");
                setDesc(task?.desc ?? "");
                setPriority(task?.priority ?? 2);
              }
        }
      />
    </>
  );
  const notices = (
    <>
      {changed && !error && (
        <p role="status" className={styles.quiet}>
          This task was updated elsewhere. Reload to edit its latest version.
        </p>
      )}
      {(reloadError || error) && <ErrorNotice error={reloadError || error} />}
      {task && (changed || conflict || reloadError) && (
        <Button type="button" disabled={busy} onClick={reload}>
          Reload latest task
        </Button>
      )}
    </>
  );
  return (
    <>
      <form
        className={presentation === "page" ? styles.pageEditor : undefined}
        id={id}
        onSubmit={(event) => {
          event.preventDefault();
          void mutate();
        }}
      >
        {presentation === "page" ? (
          <TaskDetailPanels
            controls={
              <Form
                as="fieldset"
                columns={2}
                className={detailStyles.controls}
                disabled={busy}
                aria-label="Task fields"
              >
                {priorityField}
                {statusField}
                {taskIdField}
                {aliasField}
              </Form>
            }
            metadata={
              task && (
                <TaskMetadata
                  task={task}
                  project={project}
                  owner={owner}
                  source={source}
                  issue={issue}
                />
              )
            }
            title={titleField}
            description={descriptionField}
            actions={actions}
            notices={notices}
          />
        ) : (
          <Card
            className={task ? styles.taskHeader : undefined}
            data-priority={task ? priority : undefined}
            style={
              task
                ? ({
                    "--task-priority-mask": taskPriorityMask(priority),
                  } as CSSProperties)
                : undefined
            }
            data-previous-run={issue === "Previous run" || undefined}
            title={task ? alias || task.alias : "Add task"}
            close={busy ? undefined : close}
            closeLabel="Close"
            footer={<CardActions>{actions}</CardActions>}
          >
            <>
              <Form
                as="fieldset"
                columns={2}
                className={task ? detailStyles.controls : undefined}
                disabled={busy}
                aria-label="Task fields"
              >
                {priorityField}
                {statusField}
                {taskIdField}
                {titleField}
                {aliasField}
                {descriptionField}
              </Form>
              {task && (
                <TaskMetadata
                  task={task}
                  project={project}
                  owner={owner}
                  source={source}
                  issue={issue}
                />
              )}
              {notices}
            </>
          </Card>
        )}
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

export function TaskDetails({
  c,
  project,
  selected,
  latest,
  sessions,
  close,
  saved,
  presentation = "dialog",
  setBusy = () => {},
  onDirtyChange,
}: {
  c: Connection;
  project: Project;
  selected?: string;
  latest?: Task;
  sessions: Session[];
  close: () => void;
  saved: (task: Task) => void;
  presentation?: "dialog" | "page";
  setBusy?: (busy: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [task, setTask] = useState(latest);
  const [descriptionOptions, setDescriptionOptions] = useState(
    defaultDescriptionOptions,
  );
  const [editRevision, setEditRevision] = useState(0);
  const forceReload = useRef(false);
  useEffect(() => {
    setTask((current) => current ?? latest);
  }, [latest]);
  const detail = useRemote(
    async (signal) => {
      const force = forceReload.current;
      forceReload.current = false;
      const value = selected
        ? !force && latest
          ? latest
          : await c.tasks.get(
              {
                ref: taskLookup(project, selected),
                select: { all: true },
              },
              { signal, timeoutMs: 10000 },
            )
        : undefined;
      if (!signal.aborted) {
        setTask((current) => (!current || force ? value : current));
        if (force) setEditRevision((value) => value + 1);
      }
      return value;
    },
    [c, projectId(project), selected, !!latest],
  );
  const unavailable = detail.error ? (
    <>
      <p role="status">
        This task is unavailable. Retry or return to the task list.
      </p>
      <ErrorNotice error={detail.error} />
      <Button type="button" onClick={detail.reload}>
        Retry
      </Button>
    </>
  ) : (
    <p role="status">Loading task…</p>
  );
  return (
    <>
      {!selected || task ? (
        <TaskEditor
          key={
            task
              ? `${editRevision}:${task.dateUpdated?.seconds}:${task.dateUpdated?.nanos}`
              : "add"
          }
          c={c}
          project={project}
          task={task}
          latest={latest ?? detail.data}
          sessions={sessions}
          close={close}
          presentation={presentation}
          onDirtyChange={onDirtyChange}
          descriptionOptions={descriptionOptions}
          onDescriptionOptionsChange={setDescriptionOptions}
          saved={(value) => {
            setBusy(false);
            onDirtyChange?.(false);
            if (presentation === "page") {
              setTask(value);
              setEditRevision((revision) => revision + 1);
            }
            saved(value);
          }}
          setBusy={setBusy}
          reloadError={detail.error}
          reload={() => {
            forceReload.current = true;
            detail.reload();
          }}
        />
      ) : presentation === "page" ? (
        <Section aria-label="Task status">{unavailable}</Section>
      ) : (
        <Card
          title="Task details"
          close={presentation === "dialog" ? close : undefined}
          closeLabel="Close"
        >
          {unavailable}
        </Card>
      )}
    </>
  );
}

export function TaskDialog(props: React.ComponentProps<typeof TaskDetails>) {
  const dialogClose = useDialogClose(props.close);
  const [busy, setBusy] = useState(false);
  return (
    <AnchoredDialog
      title={props.selected ? "Task details" : "Add task"}
      centered
      open={dialogClose.open}
      close={dialogClose.dismiss}
      onClosed={dialogClose.onClosed}
      busy={busy}
    >
      <TaskDetails
        {...props}
        setBusy={setBusy}
        close={dialogClose.dismiss}
        saved={(value) => dialogClose.complete(() => props.saved(value))}
      />
    </AnchoredDialog>
  );
}
