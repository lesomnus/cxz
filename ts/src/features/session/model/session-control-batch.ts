import { create } from "@bufbuild/protobuf";
import type { Session } from "#gen/cxz/session_pb";
import { SessionRefSchema } from "#gen/cxz/session_svc_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import {
  manageSession,
  resumableStates,
  type SessionOperation,
} from "./session-actions";

export type SessionControlKind = SessionOperation | "purge";
export type SessionControlConnection = Pick<
  Connection,
  "sessions" | "store" | "drafts" | "attention" | "conversations"
>;
export type SessionTarget = {
  id: Uint8Array;
  runtimeId: string;
  name: string;
  run: string;
  creation: string;
};
export function controlTarget(s: Session): SessionTarget {
  return {
    id: s.id.slice(),
    runtimeId: s.runtimeId,
    name: s.name || s.alias || s.runtimeId,
    run: s.status?.runId ?? "",
    creation: s.clientId,
  };
}
export function canControl(s: Session, kind: SessionControlKind) {
  if (kind === "purge") return true;
  if (!s.status?.runId) return false;
  const stopped = resumableStates.has(s.status.state);
  return kind === "resume" ? stopped : kind === "stop" ? !stopped : true;
}

async function assertTarget(
  c: SessionControlConnection,
  target: SessionTarget,
) {
  const current = await c.sessions.get(
    { ref: ref(target.runtimeId), select: { all: true } },
    { timeoutMs: 20_000 },
  );
  if (
    (current.status?.runId ?? "") !== target.run ||
    (target.creation && current.clientId !== target.creation) ||
    current.id.length !== target.id.length ||
    current.id.some((value, i) => value !== target.id[i])
  )
    throw new Error(t("Session run changed. Open the menu again."));
}

export async function previewSessionPurge(
  c: SessionControlConnection,
  target: SessionTarget,
) {
  await assertTarget(c, target);
  return c.sessions.purge(
    { ref: ref(target.runtimeId), dryRun: true },
    { timeoutMs: 20_000 },
  );
}

export async function performSessionControl(
  c: SessionControlConnection,
  target: SessionTarget,
  kind: SessionControlKind,
) {
  if (kind === "purge") {
    await assertTarget(c, target);
    await c.sessions.purge(
      { ref: ref(target.runtimeId) },
      { timeoutMs: 120_000 },
    );
    c.store.apply("cxz.Session", [{ id: target.id }]);
    c.drafts.delete(target.runtimeId);
    c.attention.reset(target.runtimeId);
    c.conversations.delete(target.runtimeId);
    return;
  }
  const updated = await manageSession(
    c.sessions,
    create(SessionRefSchema, ref(target.runtimeId)),
    kind,
    target.run,
  );
  c.store.apply("cxz.Session", [{ id: updated.id, value: updated }]);
}

export type SessionBatchResult<T> = { target: SessionTarget } & (
  | { ok: true; value: T }
  | { ok: false; error: string }
);

// Bound runtime startup/deletion pressure, keep per-session outcomes, and never
// retry commands automatically after a transport failure.
export async function settleSessions<T>(
  targets: readonly SessionTarget[],
  run: (target: SessionTarget) => Promise<T>,
  settled?: () => void,
) {
  const results: SessionBatchResult<T>[] = new Array(targets.length);
  let next = 0;
  async function worker() {
    while (next < targets.length) {
      const index = next++,
        target = targets[index]!;
      try {
        results[index] = { target, ok: true, value: await run(target) };
      } catch (error) {
        results[index] = { target, ok: false, error: String(error) };
      }
      settled?.();
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(3, targets.length) }, worker),
  );
  return results;
}
