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
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { TaskStatus, type Task } from "#gen/cxz/task_pb";
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
  taskReadOnly,
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
import styles from "./tasks.module.css";
import {
  TaskAssigneeControl,
  TaskDetailPanels,
  TaskMetadata,
} from "./task-detail-panels";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import actionStyles from "#src/shared/components/resource-action.module.css";
import { TaskHistory } from "./task-history";
import {
  taskAssignmentAction,
  randomAssignee,
  taskReviewRequest,
  sendTaskReview,
} from "./task-assignment";

function TaskActionButton({
  panel,
  label,
  icon,
  description,
  ...props
}: ButtonProps & {
  panel: boolean;
  label: string;
  icon: IconName;
  description?: string;
}) {
  if (!panel) return <Button {...props}>{label}</Button>;
  return (
    <Popover content={description ?? label} placement="top">
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
  task: initialTask,
  active = true,
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
  active?: boolean;
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
  // Assignment updates its version without remounting the editor or losing drafts.
  const [task, setTask] = useState(initialTask);
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
  const [confirmation, setConfirmation] = useState<"release" | "cancel">();
  const [activeAction, setActiveAction] = useState<
    "save" | "release" | "cancel" | "assign" | "review"
  >();
  const [review, setReview] = useState<{
    task: Task;
    session: Session;
    request: ReturnType<typeof taskReviewRequest>;
  }>();
  const [reviewError, setReviewError] = useState("");
  const [reviewSent, setReviewSent] = useState("");
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
  const immutable = taskReadOnly(task?.status) || taskReadOnly(latest?.status);
  const assignmentLocked =
    immutable ||
    task?.status === TaskStatus.IN_PROGRESS ||
    latest?.status === TaskStatus.IN_PROGRESS;
  const assignment = task
    ? taskAssignmentAction(task, project, sessions)
    : { kind: "none" as const };
  async function mutate(
    action: "save" | "release" | "cancel" | "assign" = "save",
  ) {
    if (
      pending.current ||
      immutable ||
      (action === "release" && assignmentLocked)
    )
      return false;
    const assignee =
      action === "assign" && assignment.kind === "random"
        ? randomAssignee(assignment.sessions)
        : undefined;
    if (action === "assign" && (!task || !assignee || changed || conflict))
      return false;
    pending.current = true;
    busyState(true);
    setActiveAction(action);
    setBusy(true);
    setError("");
    const request = new AbortController();
    controller.current = request;
    try {
      const options = { signal: request.signal, timeoutMs: 15000 };
      const value = task
        ? action === "assign" && assignee
          ? await c.tasks.claim(
              {
                ref: taskRef(project, task),
                dateUpdated: task.dateUpdated,
                actor: {
                  session: ref(assignee.runtimeId),
                  runId: assignee.status!.runId,
                },
              },
              options,
            )
          : action === "release"
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
                  ...(action === "cancel"
                    ? { status: TaskStatus.CANCELED }
                    : {
                        name: name.trim(),
                        alias: alias.trim(),
                        desc,
                        priority,
                      }),
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
        if (action === "assign") setTask(value);
        else saved(value);
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
        setActiveAction(undefined);
        setBusy(false);
      }
    }
  }
  function activateAssignment() {
    if (busy || changed || conflict || immutable) return;
    if (assignment.kind === "random") void mutate("assign");
    else if (assignment.kind === "review" && task) {
      setReviewError("");
      setReviewSent("");
      setReview({
        task,
        session: assignment.session,
        request: taskReviewRequest(task, assignment.session),
      });
    }
  }
  async function requestReview() {
    if (!review || pending.current) return false;
    pending.current = true;
    busyState(true);
    setActiveAction("review");
    setBusy(true);
    setReviewError("");
    const request = new AbortController();
    controller.current = request;
    try {
      const receipt = await sendTaskReview(
        c,
        project,
        review.task,
        review.request,
        {
          signal: request.signal,
          timeoutMs: 15000,
        },
      );
      if (!request.signal.aborted)
        setReviewSent(
          `Task review request ${receipt.status === "queued" ? "queued for" : "sent to"} ${review.session.alias || review.session.name}.`,
        );
      return true;
    } catch (failure) {
      if (!request.signal.aborted) {
        setReviewError(String(failure));
        if (ConnectError.from(failure).code === Code.FailedPrecondition)
          setConflict(true);
      }
      return false;
    } finally {
      pending.current = false;
      if (!request.signal.aborted) {
        busyState(false);
        setActiveAction(undefined);
        setBusy(false);
      }
    }
  }
  const priorityField = !task && (
    <FormField label="Priority" fullWidth>
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
    </FormField>
  );
  const taskFields = task && (
    <>
      <ResourceFields
        columns={[2, 1]}
        variant="controls"
        rows={[
          [
            "Assigned to",
            <TaskAssigneeControl
              task={task}
              owner={owner}
              action={assignment}
              busy={busy || changed || conflict || immutable}
              loading={
                busy && (activeAction === "assign" || activeAction === "review")
              }
              activate={activateAssignment}
            />,
            issue ||
              (assignment.kind === "random" && !assignment.sessions.length
                ? "No running sessions in this project"
                : undefined),
          ],
          [
            "Priority",
            <ValueMenu
              label="Priority"
              alignItemWithTrigger
              minMenuWidth={0}
              value={String(priority)}
              dirty={priority !== (task?.priority ?? 2)}
              options={taskPriorities.map((label, value) => ({
                value: String(value),
                label,
                icon: <TaskPrioritySymbol priority={value} />,
              }))}
              choose={(value) => setPriority(Number(value))}
              disabled={busy || immutable}
            />,
          ],
        ]}
      />
      <ResourceFields
        columns={[1, 2]}
        variant="controls"
        density="compact"
        rows={[
          [
            "Status",
            <output aria-label="Status">
              <TaskStatusIcon status={task.status} />
              <span>{taskStatusName(task.status)}</span>
            </output>,
          ],
          [
            "Updated",
            <DateTime value={task.dateUpdated} showOffset={false} />,
            undefined,
            <RelativeTime value={task.dateUpdated} />,
          ],
        ]}
      />
    </>
  );
  const titleField =
    presentation === "page" ? (
      <TaskTitleField
        id={`${id}-title`}
        value={name}
        disabled={busy}
        readOnly={immutable}
        dirty={name !== (task?.name ?? "")}
        change={setName}
      />
    ) : (
      <FormField
        label="Title"
        htmlFor={`${id}-title`}
        fullWidth
        dirty={name !== (task?.name ?? "")}
      >
        <input
          id={`${id}-title`}
          autoFocus={presentation === "dialog"}
          required
          disabled={busy}
          readOnly={immutable}
          maxLength={256}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </FormField>
    );
  const aliasField = (
    <FormField
      label="Alias"
      htmlFor={`${id}-alias`}
      fullWidth
      dirty={alias !== (task?.alias ?? "")}
    >
      <input
        id={`${id}-alias`}
        disabled={busy}
        readOnly={immutable}
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
        dirty={desc !== (task?.desc ?? "")}
        disabled={busy}
        readOnly={immutable}
        change={setDesc}
        options={descriptionOptions}
        setOptions={onDescriptionOptionsChange}
      />
    ) : (
      <FormField
        label="Description"
        htmlFor={`${id}-description`}
        fullWidth
        dirty={desc !== (task?.desc ?? "")}
      >
        <textarea
          id={`${id}-description`}
          className={styles.description}
          rows={5}
          disabled={busy}
          readOnly={immutable}
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
          description={
            immutable
              ? "This task is read-only"
              : assignmentLocked
                ? "Assignment is locked while Working"
                : "Release assignment and return to Todo"
          }
          icon="sign-out"
          type="button"
          loading={busy && activeAction === "release"}
          disabled={busy || assignmentLocked}
          onClick={() => setConfirmation("release")}
        />
      )}
      {task && (
        <TaskActionButton
          panel={presentation === "page"}
          label={presentation === "page" ? "Cancel" : "Cancel task"}
          icon="task-canceled"
          type="button"
          loading={busy && activeAction === "cancel"}
          disabled={busy || immutable || conflict}
          onClick={() => setConfirmation("cancel")}
        />
      )}
      <TaskActionButton
        panel={presentation === "page"}
        label={task ? "Save" : "Confirm"}
        icon="save"
        type="submit"
        form={id}
        loading={busy && activeAction === "save"}
        disabled={
          busy ||
          immutable ||
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
      {reviewSent && (
        <p role="status" className={styles.quiet}>
          {reviewSent}
        </p>
      )}
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
              <div role="group" aria-label="Task fields">
                {taskFields}
                <Form as="fieldset" columns={2} disabled={busy}>
                  {aliasField}
                </Form>
              </div>
            }
            metadata={
              task && (
                <TaskMetadata task={task} project={project} source={source} />
              )
            }
            title={titleField}
            description={descriptionField}
            actions={actions}
            notices={notices}
            history={
              task && (
                <TaskHistory
                  c={c}
                  project={project}
                  task={task}
                  latest={latest}
                  active={active}
                />
              )
            }
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
              {taskFields}
              <Form
                as="fieldset"
                columns={2}
                disabled={busy}
                aria-label="Task fields"
              >
                {priorityField}
                {titleField}
                {aliasField}
                {descriptionField}
              </Form>
              {task && (
                <TaskMetadata task={task} project={project} source={source} />
              )}
              {notices}
            </>
          </Card>
        )}
      </form>
      {confirmation && (
        <ConfirmationDialog
          title={confirmation === "release" ? "Release task?" : "Cancel task?"}
          confirmLabel={confirmation === "release" ? "Release" : "Cancel task"}
          failureMessage={
            error ||
            "Could not update the task. Close this confirmation and reload its latest version."
          }
          close={() => setConfirmation(undefined)}
          execute={() => mutate(confirmation)}
        >
          {confirmation === "release" ? (
            <>
              Return this task to Todo and clear its assignment? This does not
              stop the agent currently working on it.
            </>
          ) : (
            <>
              Mark this task as Canceled? This does not stop the agent or its
              session.
            </>
          )}
          {dirty && <p>Unsaved edits will be discarded.</p>}
        </ConfirmationDialog>
      )}
      {review && (
        <ConfirmationDialog
          title="Request task review?"
          confirmLabel="Send"
          failureMessage={reviewError || "Could not send the review request."}
          close={() => setReview(undefined)}
          execute={requestReview}
        >
          <p>
            Send this message to {review.session.name || review.session.alias} (
            {review.session.alias})?
          </p>
          <p className={styles.reviewMessage}>{review.request.text}</p>
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
  active = true,
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
  active?: boolean;
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
          active={active}
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
