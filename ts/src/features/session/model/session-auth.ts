import type { SessionEvent } from "#gen/cxz/session_pb";
import { payload } from "./journal";

const authMessages = new Set([
  "authentication failed",
  "not logged in",
  "login required",
  "not logged in · please run /login",
  "not logged in. please run /login",
  "central codex login failed",
  "central authentication unavailable; check account login",
  "central authentication refresh failed",
  "failed to authenticate: oauth session expired and could not be refreshed",
]);
function authText(value: unknown): boolean {
  if (typeof value !== "string") return false;
  const text = value.trim().toLowerCase();
  return authMessages.has(text) || /^api error:\s*401\b/i.test(text);
}
function authError(value: unknown, depth = 0): boolean {
  if (depth > 4) return false;
  if (typeof value === "string") return authText(value);
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const p = value as Record<string, any>;
  if (p.willRetry === true) return false;
  if (
    [
      "api_error_status",
      "status",
      "status_code",
      "httpStatusCode",
      "code",
    ].some((k) => p[k] === 401)
  )
    return true;
  if (
    [p.type, p.code].some((v) =>
      [
        "authentication_error",
        "authentication_failed",
        "unauthorized",
        "invalid_api_key",
        "token_expired",
      ].includes(v),
    )
  )
    return true;
  return (
    (p.type === "result" && p.is_error === true && authText(p.result)) ||
    authText(p.message) ||
    authError(p.error, depth + 1) ||
    (Array.isArray(p.errors) && p.errors.some((v) => authError(v, depth + 1)))
  );
}

// Native failures and compact transcript metadata carry the same state. Never
// infer authentication from assistant prose, tool output, or usage counters.
export function authenticationState(e: SessionEvent): boolean | undefined {
  if (!["turn_end", "diagnostic", "stderr", "error"].includes(e.kind)) return;
  const p = payload(e);
  if (typeof p.authentication_required === "boolean")
    return p.authentication_required;
  if (e.kind === "turn_end" && e.text !== "failed") return false;
  if (p.willRetry === true) return;
  const failed = authText(e.text) || authError(p);
  return failed ? true : e.kind === "turn_end" ? false : undefined;
}
export function authenticationRequired(events: SessionEvent[], run: string) {
  for (let i = events.length - 1; i >= 0; i--) {
    const e = events[i];
    if (e.runId !== run) continue;
    const state = authenticationState(e);
    if (state !== undefined) return state;
  }
  return false;
}
