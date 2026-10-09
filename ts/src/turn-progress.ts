import type { SessionEvent, SessionStatus } from "../gen/cxz/session_pb";

export type TurnProgress = {
  runId: string;
  active: boolean;
  startedAt?: number;
  key?: string;
};
const working = (state: string) =>
  ["working", "waiting_input", "running", "waiting"].includes(state);

// Feed only the latest snapshot and native live stream, never reading pages.
export function advanceTurn(
  turn: TurnProgress,
  event: SessionEvent,
  live = false,
): TurnProgress {
  if (event.runId !== turn.runId) {
    if (!live || !event.runId) return turn;
    // The ordered live stream can cross a Resume boundary without reconnecting.
    turn = { runId: event.runId, active: false };
  }
  if (
    event.kind === "turn_end" ||
    (event.kind === "state" && !working(event.text))
  )
    return turn.active || turn.startedAt !== undefined
      ? { runId: turn.runId, active: false }
      : turn;
  if (
    event.kind === "input" ||
    (event.kind === "state" && working(event.text))
  ) {
    if (turn.active && turn.startedAt !== undefined) return turn;
    const startedAt = Number(event.timeMs) || Date.now();
    return {
      runId: turn.runId,
      active: true,
      startedAt,
      key: `${turn.runId}/${event.seq}`,
    };
  }
  return turn;
}

export function snapshotTurn(
  status: SessionStatus | undefined,
  events: SessionEvent[],
  previous?: TurnProgress,
): TurnProgress {
  const runId = status?.runId ?? "";
  let turn: TurnProgress =
    previous?.runId === runId ? previous : { runId, active: false };
  for (const event of events) {
    turn = advanceTurn(turn, event);
    // Summary history folds turn_end into the final assistant row.
    if (event.runId === turn.runId && event.response?.completionJson.length)
      turn = { runId: turn.runId, active: false };
  }
  if (!working(status?.state ?? ""))
    return { runId: turn.runId, active: false };
  if (turn.active) return turn;
  // Old/retained history may lack the start: measure from first observation.
  return {
    runId: turn.runId,
    active: true,
    startedAt: Date.now(),
    key: `${turn.runId}/observed`,
  };
}
