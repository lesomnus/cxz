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
    target: "Session 89502035…2d29",
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
