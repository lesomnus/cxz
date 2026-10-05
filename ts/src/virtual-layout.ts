import type { SessionEvent } from "../gen/cxz/session_pb";

export type RowLayout = {
  id: string;
  top: number;
  height: number;
  prompt: boolean;
};
export function messageLayout(
  events: SessionEvent[],
  sizes: Map<string, number>,
) {
  let top = 0;
  const rows = events.map((event) => {
    const id = event.seq.toString();
    // Estimates are replaced by observed heights as rows enter the overscan region.
    const height =
      sizes.get(id) ??
      (event.kind === "assistant" ? 86 : event.kind === "input" ? 76 : 44);
    const row = { id, top, height, prompt: event.kind === "input" };
    top += height;
    return row;
  });
  return { rows, total: top };
}
export function rowAt(rows: RowLayout[], offset: number) {
  let low = 0,
    high = rows.length;
  while (low < high) {
    const mid = (low + high) >>> 1;
    if (rows[mid].top + rows[mid].height <= offset) low = mid + 1;
    else high = mid;
  }
  return Math.min(low, Math.max(0, rows.length - 1));
}
export function visibleRows(rows: RowLayout[], top: number, height: number) {
  // Six messages on either side, independent of total history length.
  return {
    start: Math.max(0, rowAt(rows, top) - 6),
    end: Math.min(rows.length, rowAt(rows, top + height) + 7),
  };
}
