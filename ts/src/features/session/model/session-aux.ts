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

type Named = TurnSummary & { runId: string; turn: bigint };
const key = (runId: string, turn: bigint) => `${runId}:${turn}`;

// attachSummaries maps a transcript row to the summary of the turn that row
// ends.
//
// The server names a turn by the sequence of its turn_end record, which the TUI
// can match directly because it draws that record. This view often has no such
// row: summary history folds turn_end into the final response. So a summary
// attaches to the last row of its run at or before the sequence it names --
// which is that final response in either view.
export function attachSummaries(
  events: SessionEvent[],
  state: AuxState | undefined,
): Map<string, TurnSummary> {
  const out = new Map<string, TurnSummary>();
  if (!state) return out;
  const turns = new Map<string, Named>();
  const name = (runId: string, turn: bigint, summary: TurnSummary) =>
    turns.set(key(runId, turn), { runId, turn, ...summary });
  for (const s of state.summaries)
    if (s.text)
      name(s.runId, s.turn, { text: s.text, loading: false, failed: false });
  // The task in flight wins over what is stored for the same turn: it is the
  // newer answer, and while it runs it is the only thing that can say so.
  const current = state.current;
  if (current) {
    const asked = current.kinds.includes(AuxKind.SUMMARY);
    const result = current.results.find((r) => r.kind === AuxKind.SUMMARY);
    const running = current.state === "queued" || current.state === "running";
    if (result?.text)
      name(current.runId, current.turn, {
        text: result.text,
        loading: false,
        failed: false,
      });
    else if (asked && running)
      name(current.runId, current.turn, {
        text: "",
        loading: true,
        failed: false,
      });
    else if (asked && current.message)
      name(current.runId, current.turn, {
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
  // Oldest turn first, so that if two of them resolve to the same row -- which
  // takes a hole in the loaded history -- the newer summary is the one shown.
  for (const named of [...turns.values()].sort((a, b) =>
    a.turn < b.turn ? -1 : a.turn > b.turn ? 1 : 0,
  )) {
    let target: SessionEvent | undefined;
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
