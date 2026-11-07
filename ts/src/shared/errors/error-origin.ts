import { diagnosticText } from "#src/shared/logs/log-entry.ts";

/** Allowlisted diagnostic metadata. Never pass request bodies or socket URLs. */
export type ErrorOrigin =
  | { kind: "rpc"; service: string; method: string }
  | { kind: "http"; method: string; path: string; status?: number }
  | {
      kind: "connection";
      purpose: "provider-login" | "terminal";
      transport: "websocket" | "rpc";
      closeCode?: number;
    }
  | {
      kind: "runtime";
      operation:
        | "project-status"
        | "inventory"
        | "session-history"
        | "conversation-stream"
        | "resource";
    }
  | {
      kind: "client";
      operation: "exception" | "unhandled-rejection" | "operation";
    };
export type ErrorTarget = {
  kind:
    | "session"
    | "project"
    | "account"
    | "memory"
    | "task"
    | "auth-binding"
    | "file";
  id?: string;
  runtimeId?: string;
  alias?: string;
  name?: string;
};

const runtimeOperations = [
  "project-status",
  "inventory",
  "session-history",
  "conversation-stream",
  "resource",
];
const clientOperations = ["exception", "unhandled-rejection", "operation"];
const targetKinds = [
  "session",
  "project",
  "account",
  "memory",
  "task",
  "auth-binding",
  "file",
];
const text = (value: unknown) =>
  typeof value === "string" ? diagnosticText(value, 512) : "";
const integer = (value: unknown) =>
  typeof value === "number" &&
  Number.isInteger(value) &&
  value >= 0 &&
  value <= 65535
    ? value
    : undefined;

export function sanitizeOrigin(value: unknown): ErrorOrigin | undefined {
  if (!value || typeof value !== "object") return;
  const input = value as Record<string, unknown>;
  switch (input.kind) {
    case "rpc":
      if (text(input.service) && text(input.method))
        return {
          kind: "rpc",
          service: text(input.service),
          method: text(input.method),
        };
      return;
    case "http":
      if (text(input.method) && text(input.path))
        return {
          kind: "http",
          method: text(input.method),
          path: text(input.path).split(/[?#]/)[0],
          status: integer(input.status),
        };
      return;
    case "connection":
      if (
        (input.purpose === "provider-login" || input.purpose === "terminal") &&
        (input.transport === "websocket" || input.transport === "rpc")
      )
        return {
          kind: "connection",
          purpose: input.purpose,
          transport: input.transport,
          closeCode: integer(input.closeCode),
        };
      return;
    case "runtime":
      if (runtimeOperations.includes(input.operation as string))
        return {
          kind: "runtime",
          operation: input.operation as Extract<
            ErrorOrigin,
            { kind: "runtime" }
          >["operation"],
        };
      return;
    case "client":
      if (clientOperations.includes(input.operation as string))
        return {
          kind: "client",
          operation: input.operation as Extract<
            ErrorOrigin,
            { kind: "client" }
          >["operation"],
        };
  }
}
export function sanitizeTarget(value: unknown): ErrorTarget | undefined {
  if (!value || typeof value !== "object") return;
  const input = value as Record<string, unknown>;
  if (!targetKinds.includes(input.kind as string)) return;
  const result: ErrorTarget = { kind: input.kind as ErrorTarget["kind"] };
  for (const field of ["id", "runtimeId", "alias", "name"] as const)
    if (text(input[field])) result[field] = text(input[field]);
  return result;
}

/** Old persisted records remain readable; infer from explicit source, never message text. */
export function legacyOrigin(context: {
  source?: string;
  method?: string;
}): ErrorOrigin {
  const method = diagnosticText(context.method ?? "", 512);
  if (context.source === "rpc") {
    const split = method.lastIndexOf("/");
    if (split > 0)
      return {
        kind: "rpc",
        service: method.slice(0, split),
        method: method.slice(split + 1),
      };
    return { kind: "rpc", service: "RPC", method: method || "Request" };
  }
  if (context.source === "http") {
    if (method === "Provider login")
      return {
        kind: "connection",
        purpose: "provider-login",
        transport: "websocket",
      };
    const match = /^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+(.+)$/.exec(
      method,
    );
    if (match)
      return {
        kind: "http",
        method: match[1],
        path: match[2].split(/[?#]/)[0],
      };
    return { kind: "http", method: "HTTP", path: "—" };
  }
  if (method === "Project terminal")
    return { kind: "connection", purpose: "terminal", transport: "websocket" };
  if (context.source === "resource")
    return {
      kind: "runtime",
      operation: method === "Project runtime" ? "project-status" : "resource",
    };
  return {
    kind: "client",
    operation:
      method === "JavaScript"
        ? "exception"
        : method === "Unhandled promise"
          ? "unhandled-rejection"
          : "operation",
  };
}
