import type { ClientError } from "./client-errors";
import type { ErrorOrigin, ErrorTarget } from "./error-origin";
import { t, translateKnown } from "#src/shared/i18n/i18n.ts";
import type { Message } from "#src/shared/i18n/locales/en.ts";

const targetLabels = {
  session: "Session",
  project: "Project",
  account: "Account",
  memory: "Memory",
  task: "Task",
  "auth-binding": "Auth binding",
  file: "File",
} as const satisfies Record<ErrorTarget["kind"], Message>;
const services: Record<string, string> = {
  Session: "Session",
  Project: "Project",
  Account: "Account",
  Memory: "Memory",
  Task: "Task",
  AuthBinding: "Auth binding",
  Installation: "Installation",
  Notification: "Notification",
};
const rpcActions: Record<string, Message> = {
  Get: "Read {resource}",
  List: "List {resource}",
  Watch: "Watch {resource}",
  Create: "Create {resource}",
  Add: "Create {resource}",
  Patch: "Update {resource}",
  Update: "Update {resource}",
  Delete: "Delete {resource}",
  Remove: "Delete {resource}",
  Start: "Start {resource}",
  Stop: "Stop {resource}",
  Restart: "Restart {resource}",
  Fork: "Fork {resource}",
  Merge: "Merge {resource}",
  Usage: "Read {resource} usage",
  Metrics: "Read {resource} usage",
  Changes: "Watch {resource} changes",
  Diff: "Compare {resource} revisions",
  Versions: "List {resource} revisions",
};
const sessionActions: Record<string, Message> = {
  Transcript: "Read conversation",
  Rows: "Watch conversation history",
  Events: "Watch conversation",
  Send: "Send message",
};
const runtimeActions = {
  "project-status": "Project runtime",
  inventory: "Watch resource inventory",
  "session-history": "Watch conversation history",
  "conversation-stream": "Watch conversation",
  resource: "Monitor resource",
} as const satisfies Record<
  Extract<ErrorOrigin, { kind: "runtime" }>["operation"],
  Message
>;
const clientActions = {
  exception: "Run JavaScript",
  "unhandled-rejection": "Unhandled asynchronous operation",
  operation: "Client operation",
} as const satisfies Record<
  Extract<ErrorOrigin, { kind: "client" }>["operation"],
  Message
>;
const kindLabels = {
  rpc: "RPC",
  http: "HTTP",
  connection: "Connection",
  runtime: "Runtime",
  client: "Client",
} as const satisfies Record<ErrorOrigin["kind"], Message>;

export function shortReference(value: string) {
  return value.length > 20 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}
function targetLabel(entry: ClientError) {
  const subject = entry.subject;
  if (subject) {
    if (subject.name || subject.alias) return subject.name || subject.alias!;
    const reference = subject.runtimeId || subject.id;
    return reference
      ? `${t(targetLabels[subject.kind])} ${shortReference(reference)}`
      : t(targetLabels[subject.kind]);
  }
  // Existing local records predate structured targets. Preserve their details.
  if (entry.target) return shortReference(entry.target);
  if (entry.session) return `${t("Session")} ${shortReference(entry.session)}`;
  if (entry.project) return `${t("Project")} ${shortReference(entry.project)}`;
  return "—";
}
function operationLabel(origin: ErrorOrigin): string {
  switch (origin.kind) {
    case "rpc": {
      if (
        origin.service === "cxz.SessionService" &&
        sessionActions[origin.method]
      )
        return t(sessionActions[origin.method]);
      if (origin.service === "cxz.ProjectService" && origin.method === "Editor")
        return t("Connect editor");
      if (
        origin.service === "cxz.ProjectService" &&
        origin.method === "Terminal"
      )
        return t("Open terminal");
      const service = origin.service
        .split(".")
        .at(-1)!
        .replace(/Service$/, "");
      const resource = translateKnown(
        services[service] ?? service.replace(/([a-z])([A-Z])/g, "$1 $2"),
      );
      const action = rpcActions[origin.method];
      return action
        ? t(action, { resource })
        : `${resource} · ${origin.method.replace(/([a-z])([A-Z])/g, "$1 $2")}`;
    }
    case "http":
      if (origin.path === "/auth/logout") return t("Sign out");
      if (origin.path === "/editor/connect") return t("Connect editor");
      return t("HTTP request");
    case "connection":
      return t(
        origin.purpose === "terminal" ? "Open terminal" : "Provider login",
      );
    case "runtime":
      return t(runtimeActions[origin.operation]);
    case "client":
      return t(clientActions[origin.operation]);
    default: {
      const exhaustive: never = origin;
      return exhaustive;
    }
  }
}
export function presentError(entry: ClientError) {
  const origin = entry.origin;
  const details: [Message, string | number | undefined][] = [
    ["Source", entry.source],
    ["Code", entry.code],
    ["Method", entry.method],
    ["Target", entry.target],
    ["Connection", entry.connection],
    ["Project", entry.project],
    ["Session", entry.session],
    ["Request ID", entry.request],
  ];
  switch (origin.kind) {
    case "rpc":
      details.push(["Service", origin.service], ["RPC method", origin.method]);
      break;
    case "http":
      details.push(
        ["HTTP method", origin.method],
        ["Path", origin.path],
        ["HTTP status", origin.status],
      );
      break;
    case "connection":
      details.push(
        ["Transport", origin.transport],
        ["Close code", origin.closeCode],
      );
      break;
    case "runtime":
    case "client":
      break;
    default: {
      const exhaustive: never = origin;
      return exhaustive;
    }
  }
  if (entry.subject)
    details.push(
      ["Target kind", t(targetLabels[entry.subject.kind])],
      ["Target ID", entry.subject.id],
      ["Runtime ID", entry.subject.runtimeId],
      ["Alias", entry.subject.alias],
      ["Name", entry.subject.name],
    );
  return {
    kind: t(kindLabels[origin.kind]),
    operation: operationLabel(origin),
    target: targetLabel(entry),
    details: details.filter(([, value]) => value !== undefined && value !== ""),
  };
}
