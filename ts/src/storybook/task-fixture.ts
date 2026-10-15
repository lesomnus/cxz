import { create, clone, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { TaskSchema, TaskStatus, type Task } from "#gen/cxz/task_pb";
import { TaskService } from "#gen/cxz/task_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";

export function taskFixture(count = 9, unavailable = false) {
  const id = (value: number) => {
    const id = new Uint8Array(16);
    new DataView(id.buffer).setUint32(12, value);
    return id;
  };
  const projects = [
    create(ProjectSchema, {
      id: id(1),
      runtimeId: "project-1",
      name: "Workspace",
      alias: "workspace",
      workspace: "/workspace",
      listed: true,
      status: { state: "running" },
    }),
    create(ProjectSchema, {
      id: id(2),
      runtimeId: "project-2",
      name: "Documentation",
      alias: "docs",
      workspace: "/docs",
      listed: true,
      status: { state: "running" },
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
      listed: true,
      status: { state: "working", runId: "oak-run" },
    }),
    create(SessionSchema, {
      id: id(4),
      runtimeId: "elm",
      name: "UI refinement",
      alias: "elm",
      agent: "claude",
      project: projects[0],
      listed: true,
      status: { state: "stopped", runId: "elm-run" },
    }),
  ];
  let revision = Date.now(),
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
    "Connect the task board",
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
      priority: i === 1 ? 1 : 2,
    });
  });
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
        async *watch(request, context) {
          if (unavailable)
            throw new ConnectError("Update the runtime", Code.Unimplemented);
          while (!context.signal.aborted) {
            yield {
              items: rows
                .filter(
                  (task) =>
                    task.project?.runtimeId === request.project?.key.value,
                )
                .map((value) => ({
                  id: value.id,
                  value: clone(TaskSchema, value),
                })),
            };
            await new Promise<void>((resolve) => {
              const done = () => {
                listeners.delete(done);
                context.signal.removeEventListener("abort", done);
                resolve();
              };
              listeners.add(done);
              context.signal.addEventListener("abort", done, { once: true });
              if (context.signal.aborted) done();
            });
          }
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
            project: projects.find(
              (project) => project.runtimeId === request.project?.key.value,
            ),
          });
          rows.push(task);
          notify();
          return clone(TaskSchema, task);
        },
        patch: (request) => {
          const task = find(request);
          check(task, request);
          return update(task, {
            name: request.name ?? task.name,
            alias: request.alias ?? task.alias,
            desc: request.desc ?? task.desc,
            priority: request.priority ?? task.priority,
            status: request.status || task.status,
          });
        },
        release: (request) => {
          const task = find(request);
          check(task, request);
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
