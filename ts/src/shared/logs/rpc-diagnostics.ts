import {
  Code,
  ConnectError,
  createContextValues,
  type Interceptor,
  type Transport,
} from "@connectrpc/connect";
import { runUnaryCall, runStreamingCall } from "@connectrpc/connect/protocol";
import { Id } from "@lesomnus/payday/pdid";
import { clientLogs } from "./client-logs";
import type { LogContext, LogInput } from "./log-entry";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";

import type { ErrorTarget } from "#src/shared/errors/error-origin.ts";

export type ResourceReference = { key?: { case?: string; value?: unknown } };
type ResourceKind = "session" | "project";
type Options = {
  write?: (entry: LogInput) => void;
  resolve?: (kind: ResourceKind, ref: ResourceReference) => string | undefined;
  errors?: ClientErrors;
  describe?: (target: ErrorTarget) => ErrorTarget;
};

// Reference fields only. Never include prompts, search text, settings or headers
// in a recovery key. Different accounts/tasks/pages must not resolve each other.
function references(message: unknown, depth = 0): string {
  if (!message || typeof message !== "object" || depth > 2) return "{}";
  const body = message as Record<string, unknown>;
  const result: Record<string, unknown> = {};
  for (const name of [
    "ref",
    "project",
    "session",
    "account",
    "authBinding",
    "path",
  ] as const) {
    const value = body[name];
    if (typeof value === "string") result[name] = value.slice(0, 256);
    else if (value && typeof value === "object") {
      const key = (value as ResourceReference).key;
      if (
        key?.case &&
        (typeof key.value === "string" || key.value instanceof Uint8Array)
      )
        result[name] = [
          key.case,
          typeof key.value === "string"
            ? key.value
            : [...key.value]
                .map((byte) => byte.toString(16).padStart(2, "0"))
                .join(""),
        ];
    }
  }
  if (body.query && typeof body.query === "object") {
    const nested = references(body.query, depth + 1);
    if (nested !== "{}") result.query = nested;
  }
  return JSON.stringify(result);
}
function referenceDescription(message: unknown, service: string) {
  const refs = JSON.parse(references(message)) as Record<string, unknown>;
  return Object.entries(refs)
    .flatMap(([name, value]) => {
      if (name === "query") return [];
      const label =
        name === "ref"
          ? service.replace(/^cxz\./, "").replace(/Service$/, "")
          : name;
      return [`${label}: ${Array.isArray(value) ? value[1] : value}`];
    })
    .join(" · ");
}
export function publicUUID(ref?: ResourceReference) {
  const value = ref?.key?.value;
  return ref?.key?.case === "id" &&
    value instanceof Uint8Array &&
    value.length === 16
    ? new Id(value).toString()
    : undefined;
}

const serviceTargets: Record<string, ErrorTarget["kind"]> = {
  "cxz.SessionService": "session",
  "cxz.ProjectService": "project",
  "cxz.AccountService": "account",
  "cxz.MemoryService": "memory",
  "cxz.TaskService": "task",
  "cxz.AuthBindingService": "auth-binding",
};
function referenceTarget(
  message: unknown,
  service: string,
): ErrorTarget | undefined {
  if (!message || typeof message !== "object") return;
  const body = message as Record<string, unknown>;
  for (const field of [
    "ref",
    "session",
    "project",
    "account",
    "authBinding",
    "path",
  ]) {
    const kind =
      field === "ref"
        ? serviceTargets[service]
        : field === "authBinding"
          ? "auth-binding"
          : field === "path"
            ? "file"
            : (field as ErrorTarget["kind"]);
    if (!kind) continue;
    const value = body[field];
    if (typeof value === "string" && value)
      return {
        kind,
        name: field === "path" ? value : undefined,
        alias: field !== "path" ? value : undefined,
      };
    if (!value || typeof value !== "object") continue;
    const ref = value as ResourceReference;
    if (ref.key?.case === "id") {
      const id = publicUUID(ref);
      if (id) return { kind, id };
    }
    if (typeof ref.key?.value === "string") {
      if (ref.key.case === "runtimeId")
        return { kind, runtimeId: ref.key.value };
      if (ref.key.case === "alias") return { kind, alias: ref.key.value };
    }
  }
  if (body.query && typeof body.query === "object") {
    const query = body.query as Record<string, unknown>;
    // Query metadata is reference-only; do not recurse into arbitrary filters.
    return referenceTarget(
      {
        session: query.session,
        project: query.project,
        account: query.account,
      },
      service,
    );
  }
}

export class RPCStreamError extends TypeError {
  constructor(
    message: string,
    readonly closeCode: number,
    readonly closeReason: string,
    readonly wasClean: boolean,
  ) {
    super(message);
  }
}
function closeDetails(error: unknown) {
  for (let i = 0; i < 5 && error instanceof Error; i++, error = error.cause) {
    if (error instanceof RPCStreamError)
      return {
        closeCode: error.closeCode,
        closeReason: error.closeReason,
        wasClean: error.wasClean,
      };
  }
  return {};
}

// Inspect only references, never request bodies, frames, headers or trailers.
// In particular, streaming failures happen during iteration, after next(req)
// has returned its response headers. Wrapping only next misses those failures.
export function rpcDiagnostics(
  connection: string,
  options: Options = {},
): Interceptor {
  const failures = new Map<string, { since: number; attempts: number }>();
  const errors = options.errors ?? clientErrors();
  const sink =
    options.write ?? ((entry: LogInput) => clientLogs().write(entry));
  const write = (entry: LogInput) => {
    try {
      sink(entry);
    } catch {
      /* Logging cannot fail an RPC. */
    }
  };
  return (next) => async (request) => {
    const context: LogContext = {
      connection,
      method: `${request.service.typeName}/${request.method.name}`,
      request: crypto.randomUUID(),
    };
    const started = Date.now();
    let connected = false;
    let target = "";
    let targetDescription = "";
    let subject: ErrorTarget | undefined;
    const identify = (message: unknown) => {
      const refs = references(message);
      // Bidirectional streams name their target in the first frame; later
      // input/ack frames must retain that identity.
      if (!target || refs !== "{}") {
        target = refs;
        subject = referenceTarget(message, request.service.typeName);
        targetDescription = referenceDescription(
          message,
          request.service.typeName,
        );
      }
      if (!message || typeof message !== "object") return;
      const body = message as {
        ref?: ResourceReference;
        project?: ResourceReference;
      };
      const kind =
        request.service.typeName === "cxz.SessionService"
          ? "session"
          : request.service.typeName === "cxz.ProjectService"
            ? "project"
            : undefined;
      if (kind && body.ref)
        context[kind] =
          publicUUID(body.ref) ?? options.resolve?.(kind, body.ref);
      if (body.project)
        context.project =
          publicUUID(body.project) ??
          options.resolve?.("project", body.project);
    };
    const key = () => JSON.stringify([connection, context.method, target]);
    const retry = () => {
      const failure = failures.get(key());
      if (failure)
        write({
          ...context,
          level: "info",
          category: "rpc",
          message: "Retrying RPC",
          details: {
            attempt: failure.attempts + 1,
            disconnectedMs: Date.now() - failure.since,
          },
        });
    };
    const success = () => {
      if (connected || request.signal.aborted) return;
      connected = true;
      const failure = failures.get(key());
      if (request.stream || failure)
        write({
          ...context,
          level: "info",
          category: "rpc",
          message: failure
            ? "RPC connection recovered"
            : "RPC stream connected",
          details: {
            durationMs: Date.now() - started,
            disconnectedMs: failure ? Date.now() - failure.since : undefined,
          },
        });
      failures.delete(key());
      errors.resolve(key());
    };
    const failed = (error: unknown) => {
      if (
        request.signal.aborted ||
        ConnectError.from(error).code === Code.Canceled
      )
        return;
      const previous = failures.get(key());
      failures.set(key(), {
        since: previous?.since ?? Date.now(),
        attempts: (previous?.attempts ?? 0) + 1,
      });
      if (failures.size > 256) failures.delete(failures.keys().next().value!);
      const failure = ConnectError.from(error);
      errors.report(key(), failure, {
        ...context,
        source: "rpc",
        origin: {
          kind: "rpc",
          service: request.service.typeName,
          method: request.method.name,
        },
        subject: subject ? (options.describe?.(subject) ?? subject) : undefined,
        code: Code[failure.code],
        target: targetDescription,
      });
      write({
        ...context,
        level: request.stream ? "warn" : "error",
        category: "rpc",
        message: request.stream
          ? "RPC stream disconnected"
          : "RPC request failed",
        details: {
          code: Code[failure.code],
          error: failure.rawMessage,
          durationMs: Date.now() - started,
          ...closeDetails(error),
        },
      });
    };
    try {
      let outgoing = request;
      if (request.stream) {
        const messages = request.message;
        outgoing = {
          ...request,
          message: (async function* () {
            for await (const message of messages) {
              identify(message);
              retry();
              yield message;
            }
          })(),
        };
      } else {
        identify(request.message);
        retry();
      }
      const response = await next(outgoing);
      if (!response.stream) {
        success();
        return response;
      }
      return {
        ...response,
        message: (async function* () {
          try {
            for await (const message of response.message) {
              success();
              yield message;
            }
            if (!request.signal.aborted)
              write({
                ...context,
                level: "info",
                category: "rpc",
                message: "RPC stream ended",
                details: { durationMs: Date.now() - started },
              });
          } catch (error) {
            failed(error);
            throw error;
          }
        })(),
      };
    } catch (error) {
      failed(error);
      throw error;
    }
  };
}

/** Observe every transport, including local router fixtures and upload transports. */
export function diagnosticTransport(
  transport: Transport,
  connection: string,
  options: Options = {},
): Transport {
  const interceptors = [rpcDiagnostics(connection, options)];
  return {
    unary(method, signal, timeoutMs, header, input, contextValues) {
      return runUnaryCall({
        signal,
        interceptors,
        req: {
          stream: false,
          service: method.parent,
          method,
          url: `${connection}/${method.parent.typeName}/${method.name}`,
          requestMethod: "POST",
          header: new Headers(header),
          message: input,
          contextValues: contextValues ?? createContextValues(),
        },
        next: (request) =>
          transport.unary(
            method,
            request.signal,
            timeoutMs,
            request.header,
            request.message,
            request.contextValues,
          ),
      });
    },
    stream(method, signal, timeoutMs, header, input, contextValues) {
      return runStreamingCall({
        signal,
        interceptors,
        req: {
          stream: true,
          service: method.parent,
          method,
          url: `${connection}/${method.parent.typeName}/${method.name}`,
          requestMethod: "POST",
          header: new Headers(header),
          message: input,
          contextValues: contextValues ?? createContextValues(),
        },
        next: (request) =>
          transport.stream(
            method,
            request.signal,
            timeoutMs,
            request.header,
            request.message,
            request.contextValues,
          ),
      });
    },
  };
}
