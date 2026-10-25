import { projectFixtureId, matchesProject } from "./project-id";
import { projectId } from "#src/shared/api/project-id.ts";
import { clone, create, isFieldSet } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { ProjectSchema, ContainerState } from "#gen/cxz/project_pb";
import {
  ProjectService,
  ProjectPatchRequestSchema,
} from "#gen/cxz/project_svc_pb";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
import { AccountService } from "#gen/cxz/account_svc_pb";
import { AuthBindingService } from "#gen/cxz/auth_binding_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import {
  SessionService,
  SessionPatchRequestSchema,
} from "#gen/cxz/session_svc_pb";

export function projectFixture(
  unavailable = false,
  stopped = false,
  sessionCount = 2,
  failure: "" | "control" | "preview" | "trust" = "",
) {
  const now = Date.now();
  const projects = ["Web workspace", "API workspace"].map((name, i) =>
    create(ProjectSchema, {
      id: projectFixtureId(i + 1),
      alias: i ? "api" : "web",
      name,
      dateErased: i ? timestampFromDate(new Date(now - 1000)) : undefined,
      workspace: "/host/workspace",
      config: ".devcontainer/devcontainer.json",
      desc: i
        ? "Backend services and API contracts"
        : "Frontend and design system",
      status: {
        containerState: stopped
          ? ContainerState.STOPPED
          : ContainerState.RUNNING,
        remoteWorkspace: "/workspace",
        remoteUser: "developer",
        containerId: `container-${i + 1}`,
        provisionState: "ready",
      },
      dateCreated: timestampFromDate(new Date(now - 86400000)),
      dateUpdated: timestampFromDate(new Date(now - 60000)),
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
        id: Uint8Array.from({ length: 16 }, (_, i) =>
          i === 0 ? project.id[0]! : i === 15 ? index + 1 : 0,
        ),
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
  const calls: {
    method: string;
    project?: string;
    session?: string;
    dryRun?: boolean;
    confirmed?: boolean;
    signal: AbortSignal;
  }[] = [];
  const listeners = new Set<() => void>();
  const publish = () => listeners.forEach((listener) => listener());
  let run = 0;
  const transport = createRouterTransport((router) => {
    router.service(AccountService, {
      list: (_request, context) => {
        calls.push({ method: "accounts", signal: context.signal });
        return {
          items: [
            {
              id: new Uint8Array([11]),
              alias: "work",
              name: "Work account",
              agent: "codex",
            },
          ],
        };
      },
    });
    router.service(ProjectService, {
      // A configuration that asks for something dangerous is refused until the
      // start says yes to it, the way internal/configtrust refuses it.
      start: (request, context) => {
        const index = projects.findIndex(
          (p) =>
            request.ref?.key.case === "id" &&
            p.id.every(
              (b, i) => b === (request.ref!.key.value as Uint8Array)[i],
            ),
        );
        if (index < 0)
          throw new ConnectError("Project not found", Code.NotFound);
        calls.push({
          method: request.trustConfig
            ? "project.start.trusted"
            : "project.start",
          project: projectId(projects[index]!),
          signal: context.signal,
        });
        if (failure === "trust" && !request.trustConfig) {
          const error = new ConnectError(
            [
              "devcontainer configuration requires explicit trust:",
              'configuration "/host/workspace/.devcontainer/docker-compose.yaml":  - "/services/dev/privileged": privileged container requested',
              "inspect these settings and explicitly pass --trust-config if trusted (configuration values omitted)",
            ].join("\n"),
            Code.FailedPrecondition,
          );
          error.details = [
            {
              type: "google.rpc.ErrorInfo",
              value: new BinaryWriter()
                .tag(1, WireType.LengthDelimited)
                .string("TRUST_REQUIRED")
                .tag(2, WireType.LengthDelimited)
                .string("cxz.config")
                .finish(),
            },
          ];
          throw error;
        }
        const updated = clone(ProjectSchema, projects[index]!);
        updated.status!.containerState = ContainerState.RUNNING;
        updated.dateUpdated = timestampFromDate(new Date());
        projects[index] = updated;
        publish();
        return updated;
      },
      stop: (request, context) => {
        const index = projects.findIndex(
          (p) =>
            request.ref?.key.case === "id" &&
            p.id.every(
              (b, i) => b === (request.ref!.key.value as Uint8Array)[i],
            ),
        );
        if (index < 0)
          throw new ConnectError("Project not found", Code.NotFound);
        calls.push({
          method: "project.stop",
          project: projectId(projects[index]!),
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError(
            "Project stop is unavailable",
            Code.Unavailable,
          );
        const updated = clone(ProjectSchema, projects[index]!);
        updated.status!.containerState = ContainerState.STOPPED;
        updated.dateUpdated = timestampFromDate(new Date());
        projects[index] = updated;
        for (let i = 0; i < sessions.length; i++) {
          if (projectId(sessions[i]!.project) !== projectId(updated)) continue;
          const session = clone(SessionSchema, sessions[i]!);
          session.status!.state = "stopped";
          session.status!.pending = [];
          session.project = updated;
          sessions[i] = session;
        }
        publish();
        return updated;
      },
      erase: (request, context) => {
        const index = projects.findIndex(
          (p) =>
            request.key.case === "id" &&
            p.id.every((b, i) => b === (request.key.value as Uint8Array)[i]),
        );
        if (index < 0)
          throw new ConnectError("Project not found", Code.NotFound);
        const project = clone(ProjectSchema, projects[index]!);
        calls.push({
          method: "project.erase",
          project: projectId(project),
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError(
            "Project removal is unavailable",
            Code.Unavailable,
          );
        projects.splice(index, 1);
        for (let i = sessions.length - 1; i >= 0; i--)
          if (projectId(sessions[i]!.project) === projectId(project))
            sessions.splice(i, 1);
        publish();
        return { erased: true };
      },
      get: (request) => {
        const project = projects.find((p) =>
          request.ref?.key.case === "id"
            ? p.id.every(
                (b, i) => b === (request.ref!.key.value as Uint8Array)[i],
              )
            : matchesProject(
                p,
                request.ref,
                `project-${projects.indexOf(p) + 1}`,
              ),
        );
        if (!project)
          throw new ConnectError("Project not found", Code.NotFound);
        return clone(ProjectSchema, project);
      },
      patch: (request, context) => {
        const index = projects.findIndex(
          (p) =>
            request.ref?.key.case === "id" &&
            p.id.every(
              (b, i) => b === (request.ref!.key.value as Uint8Array)[i],
            ),
        );
        if (index < 0)
          throw new ConnectError("Project not found", Code.NotFound);
        if (unavailable)
          throw new ConnectError("Project is unavailable", Code.Unavailable);
        const project = projects[index]!;
        if (
          request.dateUpdated?.seconds !== project.dateUpdated?.seconds ||
          request.dateUpdated?.nanos !== project.dateUpdated?.nanos
        )
          throw new ConnectError(
            "Project changed. Refresh and try again.",
            Code.FailedPrecondition,
          );
        const updated = clone(ProjectSchema, project);
        for (const field of ["name", "alias", "desc"] as const) {
          if (!isFieldSet(request, ProjectPatchRequestSchema.field[field]))
            continue;
          if (
            field === "alias" &&
            projects.some((p, n) => n !== index && p.alias === request.alias)
          )
            throw new ConnectError(
              "Project alias is already in use",
              Code.AlreadyExists,
            );
          updated[field] = request[field];
        }
        updated.dateUpdated = timestampFromDate(new Date());
        projects[index] = updated;
        calls.push({
          method: "project.patch",
          project: projectId(project),
          signal: context.signal,
        });
        publish();
        return clone(ProjectSchema, updated);
      },
      async *download(request, context) {
        const project = projects.find((p) =>
          matchesProject(p, request.ref, `project-${projects.indexOf(p) + 1}`),
        );
        if (!project)
          throw new ConnectError("Project not found", Code.NotFound);
        calls.push({
          method: "download",
          project: projectId(project),
          signal: context.signal,
        });
        if (unavailable)
          throw new ConnectError("Workspace is unavailable", Code.Unavailable);
        const data = new TextEncoder().encode(
          project === projects[1]
            ? "ref: refs/heads/api-main\n"
            : "ref: refs/heads/ui/dashboard\n",
        );
        yield { data, totalSize: BigInt(data.length) };
      },
    });
    router.service(SessionService, {
      add: (request, context) => {
        const project = projects.find((p) =>
          matchesProject(
            p,
            request.project,
            `project-${projects.indexOf(p) + 1}`,
          ),
        );
        if (!project)
          throw new ConnectError("Project not found", Code.NotFound);
        if (unavailable)
          throw new ConnectError(
            "Session creation is unavailable",
            Code.Unavailable,
          );
        const prior = sessions.find((s) => s.clientId === request.clientId);
        if (prior) return clone(SessionSchema, prior);
        const number = sessions.length + 1;
        const session = create(SessionSchema, {
          id: Uint8Array.from({ length: 16 }, (_, i) =>
            i === 0 ? project.id[0]! : i === 15 ? number : 0,
          ),
          runtimeId: `${project.alias}-created-${number}`,
          alias: `new-${number}`,
          name: request.name,
          clientId: request.clientId,
          model: request.model,
          project,
          account: { id: new Uint8Array([11]) },
          agent: "codex",
          status: { state: "idle", runId: "created-run" },
        });
        calls.push({
          method: "add",
          session: session.runtimeId,
          project: projectId(project),
          signal: context.signal,
        });
        sessions.push(session);
        publish();
        return clone(SessionSchema, session);
      },
      get: (request, context) => {
        const id = String(request.ref?.key.value);
        calls.push({ method: "get", session: id, signal: context.signal });
        const session = sessions.find((s) => s.runtimeId === id);
        if (!session)
          throw new ConnectError("Session not found", Code.NotFound);
        return clone(SessionSchema, session);
      },
      stop: (request, context) => control("stop", request, context.signal),
      resume: (request, context) => control("resume", request, context.signal),
      patch: (request, context) => {
        const id = String(request.ref?.key.value);
        calls.push({ method: "patch", session: id, signal: context.signal });
        const index = sessions.findIndex((s) => s.runtimeId === id);
        if (index < 0)
          throw new ConnectError("Session not found", Code.NotFound);
        if (unavailable)
          throw new ConnectError("Session is unavailable", Code.Unavailable);
        const updated = clone(SessionSchema, sessions[index]!);
        if (isFieldSet(request, SessionPatchRequestSchema.field.alias)) {
          if (
            sessions.some(
              (s) =>
                s.runtimeId !== id &&
                projectId(s.project) === projectId(updated.project) &&
                s.alias === request.alias,
            )
          )
            throw new ConnectError(
              "Session alias is already in use",
              Code.AlreadyExists,
            );
          updated.alias = request.alias;
        }
        if (isFieldSet(request, SessionPatchRequestSchema.field.name))
          updated.name = request.name;
        sessions[index] = updated;
        publish();
        return updated;
      },
      purge: (request, context) => {
        const id = String(request.ref?.key.value);
        calls.push({
          method: "purge",
          session: id,
          dryRun: request.dryRun,
          signal: context.signal,
        });
        const index = sessions.findIndex((s) => s.runtimeId === id);
        if (index < 0)
          throw new ConnectError("Session not found", Code.NotFound);
        if (
          unavailable ||
          (failure === "preview" &&
            request.dryRun &&
            sessions[index]!.alias === "elm") ||
          (failure === "control" &&
            !request.dryRun &&
            sessions[index]!.alias === "elm")
        )
          throw new ConnectError(
            "Simulated session action failure",
            Code.Unavailable,
          );
        if (!request.dryRun) {
          sessions.splice(index, 1);
          publish();
        }
        return {
          ref: request.ref,
          dryRun: request.dryRun,
          targets: [
            {
              kind: "conversation",
              path: `/storybook/${id}/journal`,
              files: 2,
              bytes: 4096n,
            },
          ],
          retained: ["Project workspace", "Account settings"],
        };
      },
    });
    router.service(AuthBindingService, {
      add: (request, context) => {
        calls.push({
          method: "binding",
          project: String(request.project?.key.value),
          signal: context.signal,
        });
        return { bindingId: "storybook-binding" };
      },
    });
    function control(
      kind: "stop" | "resume",
      request: { ref?: { key: { value?: unknown } }; runId: string },
      signal: AbortSignal,
    ) {
      const id = String(request.ref?.key.value);
      calls.push({ method: kind, session: id, signal });
      const index = sessions.findIndex((s) => s.runtimeId === id);
      if (index < 0) throw new ConnectError("Session not found", Code.NotFound);
      if (
        unavailable ||
        (failure === "control" && sessions[index]!.alias === "elm")
      )
        throw new ConnectError(
          "Simulated session action failure",
          Code.Unavailable,
        );
      const updated = clone(SessionSchema, sessions[index]!);
      if (updated.status!.runId !== request.runId)
        throw new ConnectError("Session run changed", Code.FailedPrecondition);
      updated.status!.state = kind === "stop" ? "stopped" : "idle";
      if (kind === "resume") updated.status!.runId = `resumed-${++run}`;
      updated.status!.pending = [];
      sessions[index] = updated;
      publish();
      return updated;
    }
  });
  const c = new Connection(
    `${location.origin}/storybook-projects/${crypto.randomUUID()}`,
    transport,
  );
  return {
    c,
    projects,
    sessions,
    calls,
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
