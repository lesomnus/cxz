import { afterEach, expect, it, vi } from "vitest";
import { ClientErrors } from "./client-errors";
import { presentError } from "./error-presentation";
import {
  sanitizeOrigin,
  sanitizeTarget,
  type ErrorOrigin,
} from "./error-origin";
afterEach(() => vi.useRealTimers());

it("maps typed kinds to common columns without guessing from the error message", () => {
  const cases: [ErrorOrigin, string, string][] = [
    [
      { kind: "rpc", service: "cxz.SessionService", method: "Transcript" },
      "RPC",
      "Read conversation",
    ],
    [
      { kind: "http", method: "POST", path: "/editor/connect", status: 503 },
      "HTTP",
      "Connect editor",
    ],
    [
      {
        kind: "connection",
        purpose: "provider-login",
        transport: "websocket",
        closeCode: 1006,
      },
      "Connection",
      "Provider login",
    ],
    [
      { kind: "runtime", operation: "project-status" },
      "Runtime",
      "Project runtime",
    ],
    [{ kind: "client", operation: "exception" }, "Client", "Run JavaScript"],
  ];
  for (const [origin, kind, operation] of cases) {
    const errors = new ClientErrors();
    errors.report(
      "operation",
      "cxz.UnrelatedService/Delete is just message text",
      {
        origin,
        subject: {
          kind: "session",
          name: "Release review",
          id: "full-private-reference",
        },
      },
    );
    const view = presentError(errors.snapshot()[0]);
    expect(view).toMatchObject({ kind, operation, target: "Release review" });
    expect(view.details).toContainEqual([
      "Target ID",
      "full-private-reference",
    ]);
  }
});
it("uses aliases, short IDs, missing-target placeholder and a useful fallback for unknown RPC methods", () => {
  const errors = new ClientErrors();
  errors.report("a", "offline", {
    source: "rpc",
    method: "cxz.AccountService/Get",
    subject: { kind: "account", alias: "work" },
  });
  expect(presentError(errors.snapshot()[0])).toMatchObject({
    kind: "RPC",
    operation: "Read Account",
    target: "work",
  });
  errors.report("b", "offline", {
    origin: {
      kind: "rpc",
      service: "custom.FutureService",
      method: "RunOperation",
    },
    subject: { kind: "session", runtimeId: "89502035e952a2c227802d29" },
  });
  expect(presentError(errors.snapshot()[0])).toMatchObject({
    operation: "Future · Run Operation",
    target: "8950203",
  });
  errors.report("c", "offline", {
    origin: { kind: "client", operation: "unhandled-rejection" },
  });
  expect(presentError(errors.snapshot()[0]).target).toBe("—");
});
it("redacts and allowlists new metadata on report and restore; old or invalid metadata keeps its record", () => {
  vi.useFakeTimers();
  let raw = "";
  const disk = {
    getItem: () => raw,
    setItem: (_: string, value: string) => {
      raw = value;
    },
  };
  const errors = new ClientErrors({ storage: disk });
  errors.report("a", "offline", {
    source: "http",
    method: "POST /editor/connect",
    origin: {
      kind: "http",
      method: "POST",
      path: "/editor/connect?token=do-not-save",
      status: 503,
    },
    subject: { kind: "project", name: "Bearer do-not-save", id: "project-id" },
  });
  errors.flush();
  expect(raw).not.toContain("do-not-save");
  const entry = new ClientErrors({ storage: disk }).snapshot()[0];
  expect(entry.origin).toEqual({
    kind: "http",
    method: "POST",
    path: "/editor/connect",
    status: 503,
  });
  expect(entry.subject?.name).toBe("Bearer [redacted]");
  const saved = JSON.parse(raw);
  delete saved.entries[0].origin;
  delete saved.entries[0].subject;
  raw = JSON.stringify(saved);
  expect(
    new ClientErrors({ storage: disk }).snapshot()[0].origin,
  ).toMatchObject({ kind: "http", path: "/editor/connect" });
  saved.entries[0].origin = { kind: "future", password: "do-not-save" };
  saved.entries[0].subject = { kind: "future", name: "do-not-save" };
  raw = JSON.stringify(saved);
  const restored = new ClientErrors({ storage: disk });
  expect(restored.snapshot()).toHaveLength(1);
  restored.flush();
  expect(raw).not.toContain("do-not-save");
  expect(
    sanitizeOrigin({
      kind: "connection",
      purpose: "unknown",
      transport: "websocket",
    }),
  ).toBeUndefined();
  expect(
    sanitizeOrigin({
      kind: "http",
      method: "GET",
      path: "/health",
      status: -1,
    }),
  ).toEqual({
    kind: "http",
    method: "GET",
    path: "/health",
    status: undefined,
  });
  expect(
    sanitizeTarget({ kind: "account", alias: "work", password: "do-not-save" }),
  ).toEqual({ kind: "account", alias: "work" });
});

it("redacts legacy operation metadata before it is persisted into a structured origin", () => {
  const errors = new ClientErrors();
  errors.report("legacy", "offline", {
    source: "rpc",
    method: "cxz.SessionService/Read token=do-not-save",
  });
  expect(JSON.stringify(errors.snapshot())).not.toContain("do-not-save");
  expect(errors.snapshot()[0].origin).toMatchObject({
    kind: "rpc",
    method: "Read token=[redacted]",
  });
});

it("keeps actionable kind-specific values in the row model instead of replacing them with friendly labels", () => {
  const cases: [ErrorOrigin, string, string, string?][] = [
    [
      { kind: "rpc", service: "cxz.SessionService", method: "Transcript" },
      "cxz.SessionService/Transcript · NotFound",
      "Read conversation",
      "NotFound",
    ],
    [
      { kind: "http", method: "POST", path: "/editor/connect", status: 503 },
      "POST /editor/connect · 503",
      "Connect editor",
    ],
    [
      { kind: "http", method: "PUT", path: "/new-endpoint" },
      "PUT /new-endpoint",
      "HTTP request",
    ],
    [
      {
        kind: "connection",
        purpose: "provider-login",
        transport: "websocket",
        closeCode: 1006,
      },
      "Provider login · WebSocket · 1006",
      "Provider login",
    ],
    [
      {
        kind: "runtime",
        operation: "project-status",
        state: "STOPPED",
        step: "start-container",
        containerName: "cxz-web",
      },
      "Project runtime · STOPPED · start-container · cxz-web",
      "Project runtime",
    ],
    [
      {
        kind: "client",
        operation: "exception",
        name: "TypeError",
        file: "https://workspace.example.com/assets/conversation.js",
        line: 42,
        column: 7,
      },
      "TypeError · conversation.js:42:7 · Run JavaScript",
      "Run JavaScript",
    ],
  ];
  for (const [origin, diagnostic, operation, code] of cases) {
    const errors = new ClientErrors();
    errors.report("a", "offline", { origin, code });
    expect(presentError(errors.snapshot()[0])).toMatchObject({
      diagnostic,
      operation,
    });
  }
  const errors = new ClientErrors();
  errors.report("stream", "heartbeat missing", {
    origin: { kind: "runtime", operation: "session-history" },
    method: "cxz.SessionService/Rows",
  });
  expect(presentError(errors.snapshot()[0]).diagnostic).toBe(
    "cxz.SessionService/Rows",
  );
  errors.report("legacy", "offline", { source: "rpc" });
  expect(presentError(errors.snapshot()[0]).diagnostic).toBe("RPC");
});
it("persists allowlisted runtime/client evidence, redacts source URLs, and validates source positions", () => {
  vi.useFakeTimers();
  let raw = "";
  const storage = {
    getItem: () => raw,
    setItem: (_: string, value: string) => {
      raw = value;
    },
  };
  const errors = new ClientErrors({ storage });
  errors.report("runtime", "offline", {
    origin: {
      kind: "runtime",
      operation: "project-status",
      state: "STOPPED",
      step: "start-container",
      containerName: "cxz-web",
      containerId: "full-container-id",
    },
  });
  errors.report("client", "failed", {
    origin: {
      kind: "client",
      operation: "exception",
      name: "TypeError",
      file: "https://user:private-pass@host.test/app.js?token=private-query",
      line: 42,
      column: 7,
    },
  });
  errors.flush();
  expect(raw).not.toContain("private-pass");
  expect(raw).not.toContain("private-query");
  const restored = new ClientErrors({ storage });
  expect(
    restored.snapshot().find((e) => e.key === "client")!.origin,
  ).toMatchObject({
    name: "TypeError",
    file: "https://host.test/app.js",
    line: 42,
    column: 7,
  });
  expect(
    restored.snapshot().find((e) => e.key === "runtime")!.origin,
  ).toMatchObject({
    state: "STOPPED",
    step: "start-container",
    containerName: "cxz-web",
    containerId: "full-container-id",
  });
  expect(
    sanitizeOrigin({
      kind: "client",
      operation: "exception",
      line: -1,
      column: Infinity,
      secret: "do-not-store",
    }),
  ).toEqual({
    kind: "client",
    operation: "exception",
    name: undefined,
    file: undefined,
    line: undefined,
    column: undefined,
  });
  restored.flush();
});

it("omits repeated RPC resource kinds and renders only the first seven ID characters", () => {
  const errors = new ClientErrors();
  const origin = {
    kind: "rpc",
    service: "cxz.SessionService",
    method: "Transcript",
  } as const;
  const id = "d9981aa9-9b88-8ff5-a112-136c99e59881";
  const cases = [
    { subject: { kind: "session" as const, id } },
    {
      subject: {
        kind: "session" as const,
        runtimeId: "d9981aa99b888ff5a112136c",
      },
    },
    { session: id },
    { target: `Session: ${id}` },
  ];
  for (const context of cases) {
    errors.report("rpc", "offline", { origin, ...context });
    expect(presentError(errors.snapshot()[0]).target).toBe("d9981aa");
  }
  errors.report("name", "offline", {
    origin,
    subject: { kind: "session", name: "Release review", id },
  });
  expect(presentError(errors.snapshot()[0]).target).toBe("Release review");
  errors.report("alias", "offline", {
    origin,
    subject: { kind: "session", alias: "review", id },
  });
  expect(presentError(errors.snapshot()[0]).target).toBe("review");
  errors.report("kind-only", "offline", {
    origin,
    subject: { kind: "session" },
  });
  expect(presentError(errors.snapshot()[0]).target).toBe("—");
  errors.report("legacy-resource", "offline", {
    origin: { kind: "rpc", service: "cxz.NotificationService", method: "Get" },
    target: `Notification: ${id}`,
  });
  expect(presentError(errors.snapshot()[0]).target).toBe("d9981aa");
  errors.report("runtime", "offline", {
    origin: { kind: "runtime", operation: "project-status" },
    subject: { kind: "project", id },
  });
  expect(presentError(errors.snapshot()[0]).target).toBe(
    "Project d9981aa9…9881",
  );
});
