import { expect, it } from "vitest";
import {
  Code,
  ConnectError,
  createClient,
  createRouterTransport,
} from "@connectrpc/connect";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { rpcDiagnostics, RPCStreamError } from "./rpc-diagnostics";
import type { LogInput } from "./log-entry";
import { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { diagnosticTransport } from "./rpc-diagnostics";
import { AccountService } from "#gen/cxz/account_svc_pb";

it("records stream iteration failures, retries and recovery at the same public session UUID", async () => {
  const logs: LogInput[] = [];
  let attempt = 0;
  const transport = createRouterTransport(
    (router) =>
      router.service(SessionService, {
        events: async function* () {
          if (++attempt === 1)
            throw new ConnectError("connection closed", Code.Unavailable);
          yield { heartbeat: true, text: "private response text" };
        },
      }),
    {
      transport: {
        interceptors: [
          rpcDiagnostics("https://cxz.example", { write: (e) => logs.push(e) }),
        ],
      },
    },
  );
  const client = createClient(SessionService, transport);
  const id = Uint8Array.from([
    0, 0, 0, 0, 0, 0, 128, 0, 128, 8, 0, 0, 0, 0, 0, 1,
  ]);
  const read = async () => {
    for await (const _ of client.events({
      ref: { key: { case: "id", value: id } },
      clientId: "private request text",
    })) {
    }
  };
  await expect(read()).rejects.toThrow("connection closed");
  await read();
  expect(logs.map((e) => e.message)).toEqual([
    "RPC stream disconnected",
    "Retrying RPC",
    "RPC connection recovered",
    "RPC stream ended",
  ]);
  expect(
    logs.every((e) => e.session === "00000000-0000-8000-8008-000000000001"),
  ).toBe(true);
  expect(logs[0].details?.code).toBe("Unavailable");
  expect(JSON.stringify(logs)).not.toContain("private request text");
  expect(JSON.stringify(logs)).not.toContain("private response text");
});
it("does not log normal cancellation, request/response bodies or successful unary calls", async () => {
  const logs: LogInput[] = [];
  const transport = createRouterTransport(
    (router) =>
      router.service(SessionService, {
        background: () => ({
          data: new TextEncoder().encode("private response"),
        }),
        events: async function* () {
          throw new ConnectError("closed", Code.Canceled);
        },
      }),
    {
      transport: {
        interceptors: [
          rpcDiagnostics("https://cxz.example", { write: (e) => logs.push(e) }),
        ],
      },
    },
  );
  const client = createClient(SessionService, transport);
  await client.background({});
  await expect(
    (async () => {
      for await (const _ of client.events({})) {
      }
    })(),
  ).rejects.toThrow();
  expect(logs).toEqual([]);
});
it("preserves close metadata through a ConnectError cause and ignores a failing log sink", async () => {
  const entries: LogInput[] = [];
  const intercept = rpcDiagnostics("https://cxz.example", {
    write: (e) => entries.push(e),
  });
  // A transport error retains the native WebSocket close details as its cause.
  const error = new ConnectError(
    "closed",
    Code.Unavailable,
    undefined,
    undefined,
    new RPCStreamError("closed", 1012, "service restart", true),
  );
  const transport = createRouterTransport(
    (router) =>
      router.service(SessionService, {
        background: () => {
          throw error;
        },
      }),
    { transport: { interceptors: [intercept] } },
  );
  await expect(
    createClient(SessionService, transport).background({}),
  ).rejects.toThrow();
  // In-process router transport re-encodes server errors; test the actual
  // client-side cause at the interceptor boundary instead.
  const request = {
    stream: false,
    signal: new AbortController().signal,
    service: SessionService,
    method: SessionService.method.background,
    message: {},
  };
  await expect(
    intercept(async () => {
      throw error;
    })(request as never),
  ).rejects.toBe(error);
  expect(entries.at(-1)?.details).toMatchObject({
    closeCode: 1012,
    closeReason: "service restart",
    wasClean: true,
  });
  const broken = rpcDiagnostics("https://cxz.example", {
    write: () => {
      throw new Error("storage down");
    },
  });
  await expect(
    broken(async () => {
      throw error;
    })(request as never),
  ).rejects.toBe(error);
});

it("tracks unary recovery by method and target for custom transports without recording bodies", async () => {
  const errors = new ClientErrors();
  let fail = true;
  const raw = createRouterTransport((router) =>
    router.service(AccountService, {
      get: (request) => {
        if (
          fail &&
          request.ref?.key.case === "alias" &&
          request.ref.key.value === "work"
        )
          throw new ConnectError("offline", Code.Unavailable);
        return {};
      },
      status: () => ({}),
    }),
  );
  const client = createClient(
    AccountService,
    diagnosticTransport(raw, "https://host.invalid", {
      errors,
      write: () => {},
    }),
  );
  await expect(
    client.get({ ref: { key: { case: "alias", value: "work" } } }),
  ).rejects.toThrow("offline");
  await client.get({ ref: { key: { case: "alias", value: "other" } } });
  await client.status({});
  expect(errors.snapshot()[0]).toMatchObject({
    status: "active",
    origin: { kind: "rpc", service: "cxz.AccountService", method: "Get" },
    subject: { kind: "account", alias: "work" },
  });
  fail = false;
  await client.get({ ref: { key: { case: "alias", value: "work" } } });
  expect(errors.snapshot()[0].status).toBe("resolved");
});

it("keeps stream failures active during retries until the first new data, and does not resolve on cancellation", async () => {
  const errors = new ClientErrors();
  let mode = "fail";
  let release: (() => void) | undefined;
  const raw = createRouterTransport((router) =>
    router.service(SessionService, {
      events: async function* () {
        if (mode === "fail")
          throw new ConnectError("disconnected", Code.Unavailable);
        if (mode === "wait")
          await new Promise<void>((resolve) => {
            release = resolve;
          });
        yield { heartbeat: true };
      },
    }),
  );
  const client = createClient(
    SessionService,
    diagnosticTransport(raw, "https://host.invalid", {
      errors,
      write: () => {},
    }),
  );
  const request = { ref: { key: { case: "alias" as const, value: "seal" } } };
  const read = async (signal?: AbortSignal) => {
    for await (const _ of client.events(request, { signal })) {
    }
  };
  await expect(read()).rejects.toThrow("disconnected");
  const abort = new AbortController();
  abort.abort();
  await expect(read(abort.signal)).rejects.toThrow();
  expect(errors.snapshot()[0].status).toBe("active");
  mode = "wait";
  const pending = read();
  for (let i = 0; i < 20 && !release; i++)
    await new Promise((resolve) => setTimeout(resolve, 0));
  expect(errors.snapshot()[0].status).toBe("active");
  release!();
  await pending;
  expect(errors.snapshot()[0].status).toBe("resolved");
});

it("does not resolve an error when a canceled retry later returns successfully", async () => {
  const errors = new ClientErrors();
  const interceptor = rpcDiagnostics("https://host.invalid", {
    errors,
    write: () => {},
  });
  const abort = new AbortController();
  const request = {
    stream: false,
    signal: abort.signal,
    service: AccountService,
    method: AccountService.method.get,
    message: { ref: { key: { case: "alias", value: "work" } } },
  };
  await expect(
    interceptor(async () => {
      throw new ConnectError("offline", Code.Unavailable);
    })(request as never),
  ).rejects.toThrow();
  let finish!: () => void;
  const retry = interceptor(async () => {
    await new Promise<void>((resolve) => {
      finish = resolve;
    });
    return { stream: false } as never;
  })(request as never);
  abort.abort();
  finish();
  await retry;
  expect(errors.snapshot()[0].status).toBe("active");
});

it("keeps raw RPC reference identity separate from a resolved resource name", async () => {
  const errors = new ClientErrors();
  const raw = createRouterTransport((router) =>
    router.service(SessionService, {
      transcript: () => {
        throw new ConnectError("session not found", Code.NotFound);
      },
    }),
  );
  const client = createClient(
    SessionService,
    diagnosticTransport(raw, "https://host.invalid", {
      errors,
      write: () => {},
      describe: (target) => ({
        ...target,
        name: "Release review",
        id: "public-session-id",
      }),
    }),
  );
  await expect(
    client.transcript({
      ref: { key: { case: "runtimeId", value: "runtime-session-id" } },
    }),
  ).rejects.toThrow("session not found");
  const entry = errors.snapshot()[0];
  expect(entry.subject).toEqual({
    kind: "session",
    runtimeId: "runtime-session-id",
    name: "Release review",
    id: "public-session-id",
  });
  expect(entry.origin).toEqual({
    kind: "rpc",
    service: "cxz.SessionService",
    method: "Transcript",
  });
  expect(entry.key).toContain("runtime-session-id");
  expect(entry.key).not.toContain("Release review");
  expect(entry.key).not.toContain("public-session-id");
});
