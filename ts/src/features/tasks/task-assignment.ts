import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, type CallOptions } from "@connectrpc/connect";
import { Id } from "@lesomnus/payday/pdid";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import {
  SessionSendRequestSchema,
  type SessionSendRequest,
} from "#gen/cxz/session_svc_pb";
import { TaskStatus, type Task } from "#gen/cxz/task_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import { activeTask, sessionInProject, taskRef } from "./task-model";

export type TaskAssignmentAction =
  | { kind: "random"; sessions: Session[] }
  | { kind: "review"; session: Session }
  | { kind: "none" };

export function taskAssignmentAction(
  task: Task,
  project: Project,
  sessions: Session[],
): TaskAssignmentAction {
  const available = sessions.filter(
    (session) =>
      sessionInProject(session, project) &&
      !!session.status?.runId &&
      ["idle", "working", "waiting_input"].includes(session.status.state),
  );
  if (!task.assignee && task.status === TaskStatus.TODO)
    return { kind: "random", sessions: available };
  const owner = available.find(
    (session) => session.runtimeId === task.assignee,
  );
  if (activeTask(task) && owner && owner.status!.runId !== task.assigneeRun)
    return { kind: "review", session: owner };
  return { kind: "none" };
}

export const randomAssignee = (sessions: Session[], random = Math.random) =>
  sessions[Math.floor(random() * sessions.length)];

/** Capture the confirmed run, text and retry identity when opening the modal. */
export function taskReviewRequest(task: Task, session: Session) {
  return create(SessionSendRequestSchema, {
    ref: ref(session.runtimeId),
    runId: session.status?.runId,
    clientId: crypto.randomUUID(),
    text: `Please review task "${task.name}" (alias: ${task.alias}, ID: ${new Id(task.id).toString()}).
It is assigned to a previous run of this session. Use task_get to read its latest details, then assess its progress, remaining work, and appropriate next steps. Reply in this session's language. Review only; do not change the assignment or start implementation automatically.`,
  });
}

export async function sendTaskReview(
  c: Connection,
  project: Project,
  task: Task,
  request: SessionSendRequest,
  options: CallOptions,
) {
  const current = await c.tasks.get(
    { ref: taskRef(project, task), select: { all: true } },
    options,
  );
  if (
    !activeTask(current) ||
    current.assignee !== task.assignee ||
    current.assigneeRun !== task.assigneeRun ||
    current.dateUpdated?.seconds !== task.dateUpdated?.seconds ||
    current.dateUpdated?.nanos !== task.dateUpdated?.nanos
  )
    throw new ConnectError(
      "This task changed. Close the confirmation and reload the latest task before requesting a review.",
      Code.FailedPrecondition,
    );
  const receipt = await c.sessions.send(request, options);
  if (receipt.status === "rejected")
    throw new ConnectError(
      "The session rejected the review request.",
      Code.FailedPrecondition,
    );
  return receipt;
}
