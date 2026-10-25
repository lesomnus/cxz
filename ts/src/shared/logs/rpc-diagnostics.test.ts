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
