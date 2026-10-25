import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";
import { Id } from "@lesomnus/payday/pdid";
import { clientLogs } from "./client-logs";
import type { LogContext, LogInput } from "./log-entry";

export type ResourceReference = { key?: { case?: string; value?: unknown } };
type ResourceKind = "session" | "project";
type Options = {
  write?: (entry: LogInput) => void;
  resolve?: (kind: ResourceKind, ref: ResourceReference) => string | undefined;
};
export function publicUUID(ref?: ResourceReference) {
  const value = ref?.key?.value;
  return ref?.key?.case === "id" &&
    value instanceof Uint8Array &&
    value.length === 16
    ? new Id(value).toString()
    : undefined;
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
    const identify = (message: unknown) => {
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
    const key = () =>
      `${context.method}:${context.session ?? context.project ?? ""}`;
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
      if (connected) return;
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
