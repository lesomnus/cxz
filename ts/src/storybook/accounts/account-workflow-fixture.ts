import { clone, create } from "@bufbuild/protobuf";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { AccountSchema } from "#gen/cxz/account_pb";
import {
  AccountService,
  AccountAuthStatus_State as AuthState,
} from "#gen/cxz/account_svc_pb";
import { AuthBindingService } from "#gen/cxz/auth_binding_svc_pb";
import { ContainerState, ProjectSchema } from "#gen/cxz/project_pb";
import {
  SessionSchema,
  SessionEventSchema,
  type SessionEvent,
} from "#gen/cxz/session_pb";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import type {
  LoginSocket,
  LoginSocketFactory,
} from "#src/features/accounts/login-transport.ts";
import { projectFixtureId } from "../project-id";
import { responseEvent } from "../fixtures";

export type AccountScenario =
  | "add"
  | "login"
  | "signed-in"
  | "create-session"
  | "send"
  | "expired";
export type LoginOutcome = "success" | "fail-once" | "disconnect-once";

class PreviewLoginSocket implements LoginSocket {
  onopen: LoginSocket["onopen"] = null;
  onmessage: LoginSocket["onmessage"] = null;
  onerror: LoginSocket["onerror"] = null;
  onclose: LoginSocket["onclose"] = null;
  private timers = new Set<ReturnType<typeof setTimeout>>();
  private closed = false;
  private submitting = false;
  constructor(
    outcome: LoginOutcome,
    private complete: () => void,
  ) {
    this.later(() => {
      this.onopen?.(new Event("open"));
      this.frame({
        output:
          "This is a simulated provider login.\n\nEnter **demo-code** below to complete it. No real credentials are used.\n",
      });
      if (outcome === "fail-once")
        this.later(
          () =>
            this.frame({
              error:
                "The simulated provider could not complete login. Retry to continue.",
            }),
          350,
        );
      if (outcome === "disconnect-once")
        this.later(
          () => this.onclose?.(new CloseEvent("close", { code: 1006 })),
          350,
        );
    }, 250);
  }
  private later(fn: () => void, delay: number) {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      if (!this.closed) fn();
    }, delay);
    this.timers.add(timer);
  }
  private frame(value: object) {
    this.onmessage?.(
      new MessageEvent("message", { data: JSON.stringify(value) }),
    );
  }
  send(code: string) {
    if (this.closed || this.submitting) return;
    this.submitting = true;
    this.later(() => {
      if (code.trim() !== "demo-code") {
        this.frame({
          error: "Use demo-code in this preview, then retry login.",
        });
        return;
      }
      this.complete();
      this.frame({ done: true });
    }, 450);
  }
  close() {
    this.closed = true;
    for (const timer of this.timers) clearTimeout(timer);
    this.timers.clear();
  }
}

export function accountWorkflowFixture(
  scenario: AccountScenario,
  outcome: LoginOutcome,
) {
  const isSession = ["create-session", "send", "expired"].includes(scenario);
  const account = create(AccountSchema, {
    id: projectFixtureId(10),
    alias: isSession ? "work1" : "work",
    name: isSession ? "Claude work account" : "Work account",
    agent: isSession ? "claude" : "codex",
    authBackend: isSession ? "project-local-oauth" : "brokered-access-token",
    dateCreated: timestampFromDate(new Date("2026-10-20T09:00:00Z")),
  });
  const accounts = scenario === "add" ? [] : [account];
  const signedIn = new Set(scenario === "signed-in" ? [account.alias] : []);
  const authenticatedSessions = new Set<string>();
  const project = create(ProjectSchema, {
    id: projectFixtureId(1),
    alias: "workspace",
    name: "Demo workspace",
    workspace: "/workspace",
    status: { containerState: ContainerState.RUNNING },
  });
  let session = create(SessionSchema, {
    id: projectFixtureId(20),
    runtimeId: "login-preview",
    alias: "seal",
    name: "Account login workflow",
    agent: account.agent,
    project,
    account,
    clientId: "preview-creation",
    model: isSession ? "claude-sonnet" : "gpt-6.1",
    status: { state: "idle", runId: "preview-run", lastSeq: 0n },
  });
  let revision = 0,
    loginAttempts = 0,
    run = 0,
    needsLogin = isSession;
  const events: SessionEvent[] = [];
  const changes = new Set<() => void>();
  const sockets = new Set<PreviewLoginSocket>();
  const calls: string[] = [];
  const timers = new Set<ReturnType<typeof setTimeout>>();
  const notify = () => {
    revision++;
    for (const fn of changes) fn();
  };
  const wait = (seen: number, signal: AbortSignal) =>
    new Promise<void>((resolve) => {
      if (seen !== revision || signal.aborted) return resolve();
      const done = () => {
        clearTimeout(timer);
        changes.delete(done);
        signal.removeEventListener("abort", done);
        resolve();
      };
      const timer = setTimeout(done, 10000);
      changes.add(done);
      signal.addEventListener("abort", done, { once: true });
    });
  const append = (kind: string, text: string, payload?: object) => {
    const event = create(SessionEventSchema, {
      seq: BigInt(events.length + 1),
      runId: session.status!.runId,
      kind,
      text,
      timeMs: BigInt(Date.now()),
      payload: payload
        ? new TextEncoder().encode(JSON.stringify(payload))
        : undefined,
    });
    events.push(event);
    session.status!.lastSeq = event.seq;
    notify();
    return event;
  };
  function failAuthentication() {
    append(
      "error",
      "Failed to authenticate: OAuth session expired and could not be refreshed",
      { authentication_required: true },
    );
    append("turn_end", "failed", { authentication_required: true });
    session.status!.state = "failed";
    notify();
  }
  if (scenario === "expired") {
    append("input", "Please continue reviewing the workspace.");
    failAuthentication();
  }
  const status = (alias: string) => {
    const item = accounts.find((a) => a.alias === alias);
    if (!item) throw new ConnectError("Account not found", Code.NotFound);
    const scoped = item.agent === "claude",
      ready = signedIn.has(alias);
    return {
      state: scoped
        ? AuthState.SESSION_SCOPED
        : ready
          ? AuthState.READY
          : AuthState.LOGIN_REQUIRED,
      scope: scoped ? "session" : "account",
      canLogin: !scoped && !ready,
      canLogout: !scoped && ready,
      credentialPresent: ready,
      vendorVerified: false,
      message: scoped
        ? "Login belongs to each session. Manage it from that session's details."
        : ready
          ? "Saved login is present. Provider validity has not been checked."
          : "Log in to this account to use it.",
    };
  };
  function loginRequired(key: string) {
    const writer = new BinaryWriter()
      .tag(1, WireType.LengthDelimited)
      .string("PROJECT_LOGIN_REQUIRED")
      .tag(2, WireType.LengthDelimited)
      .string("cxz.auth");
    for (const [name, value] of [
      ["account", account.alias],
      ["session_key", key],
    ])
      writer
        .tag(3, WireType.LengthDelimited)
        .bytes(
          new BinaryWriter()
            .tag(1, WireType.LengthDelimited)
            .string(name!)
            .tag(2, WireType.LengthDelimited)
            .string(value!)
            .finish(),
        );
    const error = new ConnectError(
      `account ${account.alias} needs Claude subscription login`,
      Code.FailedPrecondition,
    );
    error.details = [{ type: "google.rpc.ErrorInfo", value: writer.finish() }];
    return error;
  }
  const c = new Connection(
    `https://account-preview.invalid/${crypto.randomUUID()}`,
    createRouterTransport((router) => {
      router.service(AccountService, {
        list: () => ({ items: accounts }),
        add: (request) => {
          if (accounts.some((a) => a.alias === request.alias))
            throw new ConnectError(
              "Account already exists",
              Code.AlreadyExists,
            );
          const item = create(AccountSchema, {
            id: projectFixtureId(accounts.length + 30),
            alias: request.alias,
            name: request.name || request.alias,
            agent: request.agent,
            authBackend:
              request.agent === "claude"
                ? "project-local-oauth"
                : "brokered-access-token",
            dateCreated: timestampFromDate(new Date()),
          });
          accounts.push(item);
          notify();
          return item;
        },
        status: (request) =>
          status(request.key.case === "alias" ? request.key.value : ""),
        logout: (request) => {
          const alias = request.key.case === "alias" ? request.key.value : "";
          signedIn.delete(alias);
          notify();
          return status(alias);
        },
        models: () => ({ models: [] }),
      });
      router.service(AuthBindingService, {
        add: () => ({ bindingId: "preview-binding" }),
      });
      router.service(SessionService, {
        get: () => clone(SessionSchema, session),
        async *watch(_request, context) {
          while (!context.signal.aborted) {
            const seen = revision;
            yield {
              items: [{ id: session.id, value: clone(SessionSchema, session) }],
            };
            await wait(seen, context.signal);
          }
        },
        add: (request) => {
          calls.push(`create:${request.clientId}`);
          if (!authenticatedSessions.has(request.clientId))
            throw loginRequired(request.clientId);
          session = create(SessionSchema, {
            ...session,
            clientId: request.clientId,
            name: request.name || session.name,
            model: request.model || session.model,
          });
          session.status!.state = "idle";
          needsLogin = false;
          notify();
          return clone(SessionSchema, session);
        },
        stop: () => {
          calls.push("stop");
          session.status!.state = "stopped";
          notify();
          return clone(SessionSchema, session);
        },
        resume: (request) => {
          calls.push("resume");
          if (needsLogin) throw loginRequired(session.clientId);
          if (request.runId !== session.status!.runId)
            throw new ConnectError(
              "Session run changed",
              Code.FailedPrecondition,
            );
          session.status!.state = "idle";
          session.status!.runId = `resumed-${++run}`;
          notify();
          return clone(SessionSchema, session);
        },
        transcript: (request) => ({
          events: events.filter(
            (e) =>
              ["input", "assistant", "error"].includes(e.kind) &&
              (!request.beforeSeq || e.seq < request.beforeSeq),
          ),
          metadata: events.filter((e) => e.kind === "turn_end"),
          snapshotSeq: session.status!.lastSeq,
        }),
        history: (request) => ({
          events: events.filter((e) => e.seq > request.afterSeq),
        }),
        async *events(request, context) {
          let after = request.afterSeq;
          while (!context.signal.aborted) {
            const seen = revision;
            for (const e of events.filter((e) => e.seq > after)) {
              after = e.seq;
              yield e;
            }
            yield { heartbeat: true };
            await wait(seen, context.signal);
          }
        },
        async *rows(request, context) {
          let after = request.afterUpdated;
          while (!context.signal.aborted) {
            const seen = revision;
            for (const e of events.filter((e) => e.seq > after)) {
              after = e.seq;
              if (["input", "assistant", "error"].includes(e.kind))
                yield { event: e, updated: e.seq };
            }
            yield { updated: after };
            await wait(seen, context.signal);
          }
        },
        async *auxEvents(_request, context) {
          while (!context.signal.aborted) {
            const seen = revision;
            yield {};
            await wait(seen, context.signal);
          }
        },
        models: () => ({}),
        background: () => ({}),
        conversationStats: () => ({
          complete: true,
          total: {
            finishedTurns: 3n,
            inputTokens: 1200n,
            outputTokens: 480n,
            tokenReportedTurns: 3n,
          },
        }),
        send: (request) => {
          calls.push("send");
          append("input", request.text);
          session.status!.state = "working";
          const timer = setTimeout(() => {
            timers.delete(timer);
            if (needsLogin) failAuthentication();
            else {
              const answer = responseEvent(
                events.length + 1,
                session.agent,
                "Login is complete. This response was generated locally by the **Storybook preview**.",
                700,
              );
              answer.runId = session.status!.runId;
              answer.timeMs = BigInt(Date.now());
              events.push(answer);
              session.status!.lastSeq = answer.seq;
              append("turn_end", "completed", {
                authentication_required: false,
              });
              session.status!.state = "idle";
              notify();
            }
          }, 700);
          timers.add(timer);
          notify();
          return { status: "accepted" };
        },
      });
    }),
  );
  const connect: LoginSocketFactory = (url) => {
    calls.push(`login:${url.pathname}`);
    const attemptOutcome = ++loginAttempts === 1 ? outcome : "success";
    const socket = new PreviewLoginSocket(attemptOutcome, () => {
      const alias = url.searchParams.get("account")!;
      if (url.pathname === "/login/session") {
        authenticatedSessions.add(url.searchParams.get("key")!);
        needsLogin = false;
      } else signedIn.add(alias);
      notify();
    });
    sockets.add(socket);
    return socket;
  };
  return {
    c,
    connect,
    project,
    calls,
    get sessions() {
      return scenario === "create-session" && !authenticatedSessions.size
        ? []
        : [session];
    },
    dispose() {
      for (const timer of timers) clearTimeout(timer);
      for (const socket of sockets) socket.close();
      c.dispose();
    },
  };
}
