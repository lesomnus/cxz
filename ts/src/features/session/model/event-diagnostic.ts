import type { SessionEvent } from "#gen/cxz/session_pb";
import { payload } from "./journal";

// Error events are distinct from failed tools or turn completion statuses.
export function eventDiagnostic(event: SessionEvent) {
  if (!["diagnostic", "stderr", "error"].includes(event.kind)) return;
  const data = payload(event);
  const message = [
    data.error?.message,
    data.message,
    data.error,
    event.text,
  ].find((value) => typeof value === "string" && value.trim()) as
    | string
    | undefined;
  return {
    message: message ?? "",
    code:
      typeof data.error?.codexErrorInfo === "string"
        ? data.error.codexErrorInfo
        : undefined,
    retrying: typeof data.willRetry === "boolean" ? data.willRetry : undefined,
  };
}
