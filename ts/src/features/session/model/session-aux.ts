import { AuxKind } from "#gen/cxz/project_svc_pb";
import type { AuxState } from "#gen/cxz/session_svc_pb";
import type { SessionEvent } from "#gen/cxz/session_pb";

// A summary is one Aux result: what a model wrote about one turn, not something
// anybody said. It is drawn beside the turn it names and never merged into the
// conversation. See docs/plans/aux-api.md.
export type TurnSummary = {
  text: string;
  // Asked for and not answered yet. The row shows that it is coming rather
  // than nothing, because a summary that never arrives is indistinguishable
  // from a session that generates none.
  loading: boolean;
  // text is why it failed, in the model's place.
  failed: boolean;
};

type Named = TurnSummary & { runId: string; turn: bigint; response: bigint };
const key = (runId: string, turn: bigint) => `${runId}:${turn}`;

// attachSummaries maps a transcript row to the summary of the turn that row
// ends.
//
// responseSeq is the row, and the manager sets it because it watched the turn.
// turn names the turn_end record instead, which this view usually has no row
// for: the projection deletes it once it has folded the turn's metrics onto the
// response. Falling back to the last row at or before turn covers a manager
// that predates responseSeq -- it is right whenever the turn ends with its
// answer, and wrong when a diagnostic or an stderr row follows it.
export function attachSummaries(
  events: SessionEvent[],
  state: AuxState | undefined,
): Map<string, TurnSummary> {
  const out = new Map<string, TurnSummary>();
  if (!state) return out;
  const turns = new Map<string, Named>();
  const name = (
    runId: string,
    turn: bigint,
    response: bigint,
    summary: TurnSummary,
  ) => turns.set(key(runId, turn), { runId, turn, response, ...summary });
  for (const s of state.summaries)
    if (s.text)
      name(s.runId, s.turn, s.responseSeq, {
        text: s.text,
        loading: false,
        failed: false,
      });
  // The task in flight wins over what is stored for the same turn: it is the
  // newer answer, and while it runs it is the only thing that can say so.
  const current = state.current;
  if (current) {
    const asked = current.kinds.includes(AuxKind.SUMMARY);
    const result = current.results.find((r) => r.kind === AuxKind.SUMMARY);
    const running = current.state === "queued" || current.state === "running";
    if (result?.text)
      name(current.runId, current.turn, current.responseSeq, {
        text: result.text,
        loading: false,
        failed: false,
      });
    else if (asked && running)
      name(current.runId, current.turn, current.responseSeq, {
        text: "",
        loading: true,
        failed: false,
      });
    else if (asked && current.message)
      name(current.runId, current.turn, current.responseSeq, {
        text: current.message,
        loading: false,
        failed: true,
      });
  }
  if (!turns.size) return out;
  const byRun = new Map<string, SessionEvent[]>();
  for (const e of events) {
    const rows = byRun.get(e.runId);
    if (rows) rows.push(e);
    else byRun.set(e.runId, [e]);
  }
  const rowAt = new Map(events.map((e) => [e.seq, e]));
  // Oldest turn first, so that if two of them resolve to the same row -- which
  // takes a hole in the loaded history -- the newer summary is the one shown.
  for (const named of [...turns.values()].sort((a, b) =>
    a.turn < b.turn ? -1 : a.turn > b.turn ? 1 : 0,
  )) {
    let target = named.response ? rowAt.get(named.response) : undefined;
    if (!target && !named.response)
      for (const e of byRun.get(named.runId) ?? [])
        if (e.seq <= named.turn) target = e;
    if (target)
      out.set(target.seq.toString(), {
        text: named.text,
        loading: named.loading,
        failed: named.failed,
      });
  }
  return out;
}
