import type { SessionEvent } from "../gen/cxz/session_pb";
import { INPUT_TIME_HEIGHT, messageDate } from "./message-time";

export type RowLayout = {
  id: string;
  top: number;
  height: number;
  prompt: boolean;
  promptInset?: number;
};

// Navigation uses message positions, independent of measured pixel heights.
export const ROW_UNITS = 100;
export type RowKnot = { offset: number; fraction: number };
export type MessageMap = ReturnType<typeof messageMap>;
export function messageMap(
  rows: RowLayout[],
  origin: number,
  knots: Map<string, RowKnot>,
) {
  const points = new Map(knots);
  const fractionAt = (row: RowLayout, offset: number) => {
    const knot = points.get(row.id);
    if (!knot || knot.offset <= 0 || knot.offset >= row.height)
      return offset / row.height;
    return offset <= knot.offset
      ? (offset / knot.offset) * knot.fraction
      : knot.fraction +
          ((offset - knot.offset) / (row.height - knot.offset)) *
            (1 - knot.fraction);
  };
  return {
    rows,
    origin,
    toLogical(pixel: number) {
      if (!rows.length || pixel <= origin) return pixel;
      const index = rowAt(rows, pixel - origin),
        row = rows[index];
      return (
        origin + ROW_UNITS * (index + fractionAt(row, pixel - origin - row.top))
      );
    },
    toNative(logical: number) {
      if (!rows.length || logical <= origin) return logical;
      const position = (logical - origin) / ROW_UNITS;
      const index = Math.max(
        0,
        Math.min(rows.length - 1, Math.floor(position)),
      );
      const row = rows[index],
        fraction = position - index,
        knot = points.get(row.id);
      const offset =
        !knot || knot.offset <= 0 || knot.offset >= row.height
          ? fraction * row.height
          : fraction <= knot.fraction
            ? (fraction / knot.fraction) * knot.offset
            : knot.offset +
              ((fraction - knot.fraction) / (1 - knot.fraction)) *
                (row.height - knot.offset);
      return origin + row.top + offset;
    },
    markers: rows.flatMap((row, index) =>
      row.prompt ? [{ id: row.id, y: origin + index * ROW_UNITS + 6 }] : [],
    ),
  };
}
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
    const row = {
      id,
      top,
      height,
      prompt: event.kind === "input",
      promptInset:
        event.kind === "input" && messageDate(event.timeMs)
          ? INPUT_TIME_HEIGHT
          : 0,
    };
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
