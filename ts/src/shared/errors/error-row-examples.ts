import type { ErrorContext } from "./client-errors";
import type { ErrorOrigin } from "./error-origin";
import {
  rpcActions,
  sessionActions,
  runtimeActions,
  clientActions,
} from "./error-presentation";

type RowExample = { id: string; message: string; context: ErrorContext };
function example(
  id: string,
  message: string,
  origin: ErrorOrigin,
  context: ErrorContext = {},
): RowExample {
  return {
    id,
    message,
    context: {
      origin,
      connection: "https://workspace.example.com",
      ...context,
    },
  };
}
export const errorRowExamples: RowExample[] = [
  ...Object.keys(rpcActions).map((method) =>
    example(
      `rpc-${method}`,
      `${method} failed because the service is temporarily unavailable.`,
      { kind: "rpc", service: "cxz.MemoryService", method },
      {
        method: `cxz.MemoryService/${method}`,
        code: "Unavailable",
        subject: {
          kind: "memory",
          name: "Architecture decisions",
          id: "memory-example-id",
        },
      },
    ),
  ),
  ...Object.keys(sessionActions).map((method) =>
    example(
      `session-${method}`,
      "The conversation could not be synchronized with the server.",
      { kind: "rpc", service: "cxz.SessionService", method },
      {
        method: `cxz.SessionService/${method}`,
        code: "NotFound",
        subject: {
          kind: "session",
          name: "Release review",
          alias: "review",
          runtimeId: "89502035e952a2c227802d29",
        },
      },
    ),
  ),
  ...["Editor", "Terminal"].map((method) =>
    example(
      `project-${method}`,
      "The project container did not respond to the connection request.",
      { kind: "rpc", service: "cxz.ProjectService", method },
      {
        method: `cxz.ProjectService/${method}`,
        code: "DeadlineExceeded",
        subject: { kind: "project", name: "Web workspace", alias: "web" },
      },
    ),
  ),
  example(
    "rpc-fallback",
    "The server rejected an operation from an additional service.",
    { kind: "rpc", service: "custom.PreviewService", method: "BuildPreview" },
    {
      subject: { kind: "project", id: "d9981aa9-9b88-8ff5-a112-136c99e59881" },
    },
  ),
  example(
    "http-sign-out",
    "Sign out could not be completed. Try again when the server is available.",
    { kind: "http", method: "POST", path: "/auth/logout", status: 503 },
  ),
  example(
    "http-editor",
    "The editor connection timed out.",
    { kind: "http", method: "POST", path: "/editor/connect", status: 504 },
    { subject: { kind: "project", alias: "web" } },
  ),
  example(
    "http-fallback",
    "The file upload was rejected because the file exceeds the server limit.",
    { kind: "http", method: "POST", path: "/upload", status: 413 },
    { subject: { kind: "file", name: "design.png" } },
  ),
  example(
    "connection-login",
    "Provider login closed before authentication completed.",
    {
      kind: "connection",
      purpose: "provider-login",
      transport: "websocket",
      closeCode: 1006,
    },
    { subject: { kind: "account", alias: "work" } },
  ),
  example(
    "connection-terminal",
    "Terminal disconnected while the project container was restarting.",
    {
      kind: "connection",
      purpose: "terminal",
      transport: "websocket",
      closeCode: 1012,
    },
    { subject: { kind: "project", name: "Web workspace" } },
  ),
  example(
    "connection-terminal-rpc",
    "The in-process terminal stream ended unexpectedly.",
    { kind: "connection", purpose: "terminal", transport: "rpc" },
    { subject: { kind: "project", alias: "sandbox" } },
  ),
  ...Object.keys(runtimeActions).map((operation) => {
    const session =
      operation === "session-history" || operation === "conversation-stream";
    const method =
      operation === "inventory"
        ? "cxz.ProjectService/Watch"
        : operation === "session-history"
          ? "cxz.SessionService/Rows"
          : operation === "conversation-stream"
            ? "cxz.SessionService/Events"
            : "Project runtime";
    return example(
      `runtime-${operation}`,
      "The resource stopped reporting a healthy state.",
      {
        kind: "runtime",
        operation: operation as Extract<
          ErrorOrigin,
          { kind: "runtime" }
        >["operation"],
        ...(operation === "project-status"
          ? {
              state: "STOPPED",
              step: "start-container",
              containerName: "cxz-web",
              containerId: "a9981aa99b888ff5a112136c99e59881",
            }
          : {}),
      },
      {
        method,
        subject: session
          ? { kind: "session", name: "Release review", alias: "review" }
          : { kind: "project", name: "Web workspace" },
      },
    );
  }),
  ...Object.keys(clientActions).map((operation) =>
    example(
      `client-${operation}`,
      "The browser could not complete a local operation.",
      {
        kind: "client",
        operation: operation as Extract<
          ErrorOrigin,
          { kind: "client" }
        >["operation"],
        name:
          operation === "exception"
            ? "TypeError"
            : operation === "unhandled-rejection"
              ? "NetworkError"
              : "RangeError",
        ...(operation === "exception"
          ? {
              file: "https://workspace.example.com/assets/conversation.js",
              line: 42,
              column: 7,
            }
          : {}),
      },
      {
        method:
          operation === "exception"
            ? "JavaScript"
            : operation === "unhandled-rejection"
              ? "Unhandled promise"
              : "Render preview",
      },
    ),
  ),
];
