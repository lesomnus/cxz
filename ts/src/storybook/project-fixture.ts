import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import {
  Code,
  ConnectError,
  createClient,
  createRouterTransport,
} from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { TaskService } from "#gen/cxz/task_svc_pb";
import { TaskSchema, TaskStatus } from "#gen/cxz/task_pb";
import { TaskCatalog } from "#src/features/tasks/task-catalog.ts";

export function projectFixture(
  unavailable = false,
  stopped = false,
  latency = 0,
  sessionCount = 2,
) {
  const now = Date.now();
  const projects = ["Web workspace", "API workspace"].map((name, i) =>
    create(ProjectSchema, {
      id: new Uint8Array([i + 1]),
      runtimeId: `project-${i + 1}`,
      alias: i ? "api" : "web",
      name,
      workspace: "/host/workspace",
      desc: i
        ? "Backend services and API contracts"
        : "Frontend and design system",
      status: {
        state: stopped ? "stopped" : "running",
        remoteWorkspace: "/workspace",
        remoteUser: "developer",
        containerId: `container-${i + 1}`,
        provisionState: "ready",
      },
      dateCreated: timestampFromDate(new Date(now - 86400000)),
    }),
  );
  const event = (seq: number, kind: string, text: string, payload?: unknown) =>
    create(SessionEventSchema, {
      runId: "current-run",
      seq: BigInt(seq),
      kind,
      text,
      timeMs: BigInt(now - (10 - seq) * 60000),
      payload: payload
        ? new TextEncoder().encode(JSON.stringify(payload))
        : undefined,
    });
  const sessions = projects.flatMap((project) =>
    Array.from(
      { length: sessionCount },
      (_, index) =>
        ["Plan the next release", "Review changes"][index] ??
        `Session ${index + 1}`,
    ).map((name, index) =>
      create(SessionSchema, {
        runtimeId: `${project.alias}-${index}`,
        alias: index > 1 ? `session-${index + 1}` : index ? "elm" : "oak",
        name,
        project,
        agent: index ? "claude" : "codex",
        model: "creation-model",
        account: { id: new Uint8Array([11]) },
        dateCreated: timestampFromDate(new Date(now - 3600000)),
        dateUpdated: timestampFromDate(new Date(now - 60000)),
        status: {
          state: stopped ? "stopped" : index ? "idle" : "waiting_input",
          runId: "current-run",
          lastSeq: 9n,
          vendorId: "provider-session",
          pending:
            index || stopped
              ? []
              : [
                  event(9, "approval", "item/tool/requestUserInput", {
                    params: {
                      questions: [
                        {
                          id: "plan",
                          question: "Which release should we prepare?",
                          options: [],
                        },
                      ],
                    },
                  }),
                ],
        },
      }),
    ),
  );
  const rows = [
    event(4, "input", "Prepare the release and check the tests."),
    event(5, "assistant", "I will inspect the changes and verify the release."),
    event(6, "error", "request failed", {
      error: { message: "Provider rate limit reached; retrying shortly." },
    }),
    event(7, "assistant", "Tests passed. Which release should we prepare?"),
  ];
  const metadata = [
    event(1, "models", "", {
      effective_model: "current-model",
      effective_effort: "high",
    }),
    event(2, "usage", "account/rateLimits/updated", {
      rateLimits: { primary: { usedPercent: 25 } },
    }),
    event(3, "usage", "thread/tokenUsage/updated", {
      tokenUsage: { last: { totalTokens: 24000 }, modelContextWindow: 200000 },
    }),
  ];
  const tasks = [
    "Review the release plan",
    "Verify browser tests",
    "Prepare release notes",
  ].map((name, index) =>
    create(TaskSchema, {
      id: new Uint8Array([index + 1]),
      name,
      alias: `task-${index + 1}`,
      project: projects[0],
      status: [TaskStatus.IN_PROGRESS, TaskStatus.WAITING, TaskStatus.TODO][
        index
      ],
      assignee: index ? "" : "web-0",
      assigneeRun: "current-run",
    }),
  );
  const calls: {
    method: string;
    project?: string;
    session?: string;
    limit?: number;
    signal: AbortSignal;
  }[] = [];
  const transport = createRouterTransport((router) => {
    router.service(AccountService, {
      get: () => ({ alias: "work", name: "Work account" }),
    });
    router.service(ProjectService, {
      async *download(request, context) {
        calls.push({
          method: "download",
          project: String(request.ref?.key.value),
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError("Workspace is unavailable", Code.Unavailable);
        const data = new TextEncoder().encode(
          request.ref?.key.value === "project-2"
            ? "ref: refs/heads/api-main\n"
            : "ref: refs/heads/ui/dashboard\n",
        );
        yield { data, totalSize: BigInt(data.length) };
      },
    });
    router.service(SessionService, {
      async transcript(request, context) {
        calls.push({
          method: "transcript",
          session: String(request.ref?.key.value),
          limit: request.limit,
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError(
            "Session runtime is unavailable",
            Code.Unavailable,
          );
        if (latency)
          await new Promise<void>((resolve, reject) => {
            const aborted = () => {
              clearTimeout(timer);
              context.signal.removeEventListener("abort", aborted);
              reject(context.signal.reason);
            };
            const timer = setTimeout(() => {
              context.signal.removeEventListener("abort", aborted);
              resolve();
            }, latency);
            context.signal.addEventListener("abort", aborted, { once: true });
            if (context.signal.aborted) aborted();
          });
        return { events: rows, metadata, snapshotSeq: 9n, hasOlder: true };
      },
    });
    router.service(TaskService, {
      async *watch(request, context) {
        calls.push({
          method: "tasks",
          project: String(request.project?.key.value),
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError("Task API unavailable", Code.Unimplemented);
        yield {
          items: tasks
            .filter(
              (task) => task.project?.runtimeId === request.project?.key.value,
            )
            .map((value) => ({ id: value.id, value })),
        };
        await new Promise<void>((resolve) => {
          context.signal.addEventListener("abort", () => resolve(), {
            once: true,
          });
          if (context.signal.aborted) resolve();
        });
      },
    });
  });
  const c = {
    accounts: createClient(AccountService, transport),
    projects: createClient(ProjectService, transport),
    sessions: createClient(SessionService, transport),
    taskCatalog: new TaskCatalog(createClient(TaskService, transport)),
  };
  return { c, projects, sessions, calls };
}
