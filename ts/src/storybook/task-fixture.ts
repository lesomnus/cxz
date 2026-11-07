import { projectFixtureId, matchesProject } from "./project-id";
import {
  create,
  clone,
  isFieldSet,
  type MessageInitShape,
} from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { ContainerState, ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { TaskSchema, TaskStatus, type Task } from "#gen/cxz/task_pb";
import { TaskService, TaskPatchRequestSchema } from "#gen/cxz/task_svc_pb";
import { key } from "@lesomnus/payday/store";
import { Connection } from "#src/shared/api/connection.ts";

export function taskFixture(
  count = 9,
  unavailable = false,
  options: {
    pageSize?: number;
    pageDelay?: number;
    olderDone?: number;
    longDetails?: boolean;
  } = {},
) {
  const id = (value: number) => {
    const id = new Uint8Array(16);
    new DataView(id.buffer).setUint32(12, value);
    return id;
  };
  const projects = [
    create(ProjectSchema, {
      id: projectFixtureId(1),
      name: "Workspace",
      alias: "workspace",
      workspace: "/workspace",
      status: { containerState: ContainerState.RUNNING },
    }),
    create(ProjectSchema, {
      id: projectFixtureId(2),
      name: "Documentation",
      alias: "docs",
      workspace: "/docs",
      status: { containerState: ContainerState.RUNNING },
    }),
  ];
  const sessions = [
    create(SessionSchema, {
      id: id(3),
      runtimeId: "oak",
      name: "Task API",
      alias: "oak",
      agent: "codex",
      project: projects[0],
      status: { state: "working", runId: "oak-run" },
    }),
    create(SessionSchema, {
      id: id(4),
      runtimeId: "elm",
      name: "UI refinement",
      alias: "elm",
      agent: "claude",
      project: projects[0],
      status: { state: "stopped", runId: "elm-run" },
    }),
  ];
  const fixtureTime = Date.now();
  let revision = fixtureTime,
    serial = 100;
  const stamp = () => timestampFromDate(new Date(++revision));
  const make = (index: number, init: MessageInitShape<typeof TaskSchema>) =>
    create(TaskSchema, {
      id: id(index + 10),
      alias: `task-${index + 1}`,
      name: `Task ${index + 1}`,
      desc: "Shared project work. Keep the outcome and current status up to date.",
      project: projects[0],
      dateUpdated: stamp(),
      dateCreated: stamp(),
      priority: 2,
      ...init,
    });
  const names = [
    "Implement Task MCP tools",
    "Review project isolation",
    "Connect the task table",
    "Wait for CI",
    "Verify claim concurrency",
    "Document the agent workflow",
    "Restore interrupted work",
    "Clean up the old experiment",
    "Add browser coverage",
  ];
  const rows = Array.from({ length: count }, (_, i) => {
    const status = [
      TaskStatus.TODO,
      TaskStatus.IN_PROGRESS,
      TaskStatus.IN_PROGRESS,
      TaskStatus.WAITING,
      TaskStatus.DONE,
      TaskStatus.DONE,
      TaskStatus.IN_PROGRESS,
      TaskStatus.CANCELED,
      TaskStatus.TODO,
    ][i % 9];
    const owned = status !== TaskStatus.TODO && status !== TaskStatus.CANCELED;
    return make(i, {
      name: names[i % names.length] + (i >= names.length ? ` ${i + 1}` : ""),
      status,
      assignee: owned ? (i % 9 === 2 ? "elm" : "oak") : "",
      assigneeRun: owned
        ? i % 9 === 2
          ? "elm-run"
          : i % 9 === 6
            ? "old-oak-run"
            : "oak-run"
        : "",
      sourceSession: "oak",
      sourceRun: "oak-run",
      labels: { area: i % 2 ? "api" : "web" },
      priority: i % 5,
      dateCreated: timestampFromDate(
        new Date(fixtureTime - (count - i) * 3_600_000),
      ),
      dateUpdated: timestampFromDate(
        new Date(fixtureTime - (count - i) * 300_000),
      ),
    });
  });
  const historyStart = new Date(fixtureTime);
  if (options.longDetails && rows[2]) {
    rows[2].desc =
      "Implementation notes\n\n" +
      "Keep task navigation independent of the editor's scroll position.\n".repeat(
        50,
      );
    rows[2].labels = Object.fromEntries(
      Array.from({ length: 36 }, (_, index) => [
        `milestone-${index + 1}`,
        `Acceptance criterion ${index + 1}`,
      ]),
    );
  }
  historyStart.setHours(0, 0, 0, 0);
  historyStart.setDate(historyStart.getDate() - 2);
  for (let i = 0; i < (options.olderDone ?? 0); i++) {
    const date = timestampFromDate(
      new Date(historyStart.getTime() - i * 3_600_000),
    );
    rows.push(
      make(1000 + i, {
        alias: `completed-${i + 1}`,
        name: `Completed task ${i + 1}`,
        status: TaskStatus.DONE,
        dateCreated: date,
        dateUpdated: date,
      }),
    );
  }
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((listener) => listener());
  const update = (task: Task, patch: Partial<Task>) => {
    Object.assign(task, patch, { dateUpdated: stamp() });
    notify();
    return clone(TaskSchema, task);
  };
  const find = (request: {
    ref?: { key: { case?: string; value?: unknown } };
  }) => {
    const key = request.ref?.key;
    const task = rows.find((task) =>
      key?.case === "alias"
        ? task.alias === key.value
        : key?.case === "id" && String(task.id) === String(key.value),
    );
    if (!task) throw new ConnectError("Task not found", Code.NotFound);
    return task;
  };
  const check = (
    task: Task,
    request: { dateUpdated?: Task["dateUpdated"] },
  ) => {
    if (
      request.dateUpdated?.seconds !== task.dateUpdated?.seconds ||
      request.dateUpdated?.nanos !== task.dateUpdated?.nanos
    )
      throw new ConnectError("Task version changed", Code.FailedPrecondition);
  };
  const c = new Connection(
    "https://task-preview.invalid",
    createRouterTransport((router) => {
      router.service(TaskService, {
        async list(request, context) {
          if (unavailable)
            throw new ConnectError("Update the runtime", Code.Unimplemented);
          if (options.pageDelay)
            await new Promise<void>((resolve, reject) => {
              const finish = () => {
                context.signal.removeEventListener("abort", abort);
                resolve();
              };
              const timer = setTimeout(finish, options.pageDelay);
              const abort = () => {
                clearTimeout(timer);
                reject(new ConnectError("Canceled", Code.Canceled));
              };
              context.signal.addEventListener("abort", abort, { once: true });
            });
          const ordered = rows
            .filter(
              (task) =>
                !!task.project && matchesProject(task.project, request.project),
            )
            .sort((a, b) => {
              const age =
                Number(
                  (a.dateCreated?.seconds ?? 0n) -
                    (b.dateCreated?.seconds ?? 0n),
                ) ||
                (a.dateCreated?.nanos ?? 0) - (b.dateCreated?.nanos ?? 0) ||
                key(a.id).localeCompare(key(b.id));
              return request.reverse ? -age : age;
            });
          const start = request.after
            ? ordered.findIndex((task) => key(task.id) === request.after) + 1
            : 0;
          const size = Math.min(request.size || 100, options.pageSize || 500);
          const items = ordered.slice(start, start + size);
          return {
            items: items.map((task) => clone(TaskSchema, task)),
            next: start + size < ordered.length ? key(items.at(-1)!.id) : "",
          };
        },
        watch(request, context) {
          if (unavailable)
            throw new ConnectError("Update the runtime", Code.Unimplemented);
          const current = () =>
            rows.filter(
              (task) =>
                !!task.project && matchesProject(task.project, request.project),
            );
          const versions = () =>
            new Map(
              current().map((task) => [
                key(task.id),
                `${task.dateUpdated?.seconds}:${task.dateUpdated?.nanos}`,
              ]),
            );
          let previous = versions(),
            dirty = false;
          let wake: (() => void) | undefined;
          const changed = () => {
            dirty = true;
            wake?.();
          };
          listeners.add(changed);
          const abort = () => {
            listeners.delete(changed);
            wake?.();
          };
          context.signal.addEventListener("abort", abort, { once: true });
          context.responseHeader.set("cxz-watch-ready", "1");
          return (async function* () {
            try {
              yield {
                items: request.skipSnapshot
                  ? []
                  : current().map((value) => ({
                      id: value.id,
                      value: clone(TaskSchema, value),
                    })),
              };
              while (!context.signal.aborted) {
                if (!dirty)
                  await new Promise<void>((resolve) => {
                    wake = resolve;
                    if (context.signal.aborted) resolve();
                  });
                if (context.signal.aborted) break;
                dirty = false;
                const next = versions();
                const items = current()
                  .filter(
                    (task) =>
                      previous.get(key(task.id)) !== next.get(key(task.id)),
                  )
                  .map((value) => ({
                    id: value.id,
                    value: clone(TaskSchema, value),
                  }));
                previous = next;
                if (items.length) yield { items };
              }
            } finally {
              listeners.delete(changed);
              context.signal.removeEventListener("abort", abort);
            }
          })();
        },
        get: (request) => clone(TaskSchema, find(request)),
        add: (request) => {
          const existing = rows.find(
            (task) => task.clientId === request.clientId,
          );
          if (existing) return clone(TaskSchema, existing);
          const task = make(serial++, {
            name: request.name,
            desc: request.desc,
            alias: request.alias || `task-${serial}`,
            priority: request.priority,
            status: TaskStatus.TODO,
            clientId: request.clientId,
            project: projects.find((project) =>
              matchesProject(project, request.project),
            ),
          });
          rows.push(task);
          notify();
          return clone(TaskSchema, task);
        },
        patch: (request) => {
          const task = find(request);
          check(task, request);
          if (task.status === TaskStatus.DONE)
            throw new ConnectError(
              "Completed tasks are immutable",
              Code.FailedPrecondition,
            );
          const fields = [
            "name",
            "alias",
            "desc",
            "priority",
            "status",
          ] as const;
          return update(
            task,
            Object.fromEntries(
              fields
                .filter((field) =>
                  isFieldSet(request, TaskPatchRequestSchema.field[field]),
                )
                .map((field) => [field, request[field]]),
            ),
          );
        },
        release: (request) => {
          const task = find(request);
          check(task, request);
          if (
            task.status === TaskStatus.DONE ||
            task.status === TaskStatus.IN_PROGRESS
          )
            throw new ConnectError(
              "Task assignment is locked",
              Code.FailedPrecondition,
            );
          if (
            request.actor?.session?.key.value !== task.assignee ||
            request.actor?.runId !== task.assigneeRun
          )
            throw new ConnectError("Owner changed", Code.FailedPrecondition);
          return update(task, {
            assignee: "",
            assigneeRun: "",
            status: TaskStatus.TODO,
          });
        },
      });
    }),
  );
  return {
    c,
    projects,
    sessions,
    agentUpdate: () => {
      const task = rows[0];
      if (task)
        update(task, {
          name: "Agent updated the task",
          alias: "agent-renamed-task",
          status: TaskStatus.IN_PROGRESS,
          assignee: "oak",
          assigneeRun: "oak-run",
        });
    },
  };
}
