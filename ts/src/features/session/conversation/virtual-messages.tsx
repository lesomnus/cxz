import React, {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { useTranscriptMotion } from "./transcript-motion";
import { ActivityCardUnfolded } from "./activity-card";
import {
  messageLayout,
  messageMap,
  messageMapShift,
  ROW_UNITS,
  rowAt,
  visibleRows,
  type MessageMap,
  type RowKnot,
} from "./virtual-layout";

export function VirtualMessages({
  pane,
  events,
  follow,
  render,
  changed,
  precedingPrompt,
  onPromptChange,
}: {
  pane: React.RefObject<HTMLDivElement | null>;
  events: SessionEvent[];
  follow: React.RefObject<boolean>;
  render: (event: SessionEvent) => React.ReactNode;
  changed: (mapping: MessageMap) => void;
  precedingPrompt: SessionEvent | undefined;
  onPromptChange: (prompt: SessionEvent | undefined, peek: number) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const sizes = useRef(new Map<string, number>());
  const knots = useRef(new Map<string, RowKnot>());
  const mapping = useRef<MessageMap | null>(null);
  const [revision, setRevision] = useState(0);
  const [rowPadding, setRowPadding] = useState(0);
  const [viewport, setViewport] = useState({ top: 0, offset: 0, height: 800 });
  const layout = useMemo(
    () => messageLayout(events, sizes.current),
    [events, revision],
  );
  const previous = useRef<typeof layout | null>(null);
  const latest = useRef({ layout, follow, changed });
  latest.current = { layout, follow, changed };
  const anchor = useRef<{ id: string; offset: number } | null>(null);
  const jumpAnchor = useRef<{ id: string; top: number } | null>(null);
  const observer = useRef<ResizeObserver | null>(null);
  const width = useRef(0);
  const lastPrompt = useRef<SessionEvent | undefined>(undefined);

  function remember() {
    const el = pane.current;
    if (!el) return;
    const rows = latest.current.layout.rows;
    const preferred = jumpAnchor.current;
    const target =
      preferred && el.scrollTop === preferred.top
        ? rows.find((row) => row.id === preferred.id)
        : undefined;
    if (!target) jumpAnchor.current = null;
    const row =
      target ??
      rows[rowAt(rows, Math.max(0, el.scrollTop - root.current!.offsetTop))];
    anchor.current = row
      ? { id: row.id, offset: row.top + root.current!.offsetTop - el.scrollTop }
      : null;
    setViewport({
      top: Math.max(0, el.scrollTop - root.current!.offsetTop),
      offset: el.scrollTop - root.current!.offsetTop,
      height: el.clientHeight,
    });
  }
  useEffect(() => {
    const el = pane.current!;
    observer.current = new ResizeObserver((entries) => {
      let dirty = false;
      const mountedRow = root.current!.querySelector<HTMLElement>("[data-row]");
      if (mountedRow)
        setRowPadding(parseFloat(getComputedStyle(mountedRow).paddingTop));
      // The centered reading column can keep its width while the pane resizes.
      // Clearing heights then loses mounted measurements: those rows did not
      // resize, so ResizeObserver has no replacement entries to deliver.
      const columnWidth = root.current!.clientWidth;
      if (columnWidth !== width.current) {
        if (width.current) {
          sizes.current.clear();
          dirty = true;
        }
        width.current = columnWidth;
      }
      for (const entry of entries) {
        const node = entry.target as HTMLElement;
        if (!node.dataset.row) continue;
        const height = node.getBoundingClientRect().height;
        if (
          height > 0 &&
          Math.abs((sizes.current.get(node.dataset.row) ?? 0) - height) > 0.5
        ) {
          sizes.current.set(node.dataset.row, height);
          dirty = true;
        }
      }
      if (dirty) setRevision((n) => n + 1);
      else if (entries.some((entry) => entry.target === el)) remember();
    });
    observer.current.observe(el);
    for (const row of root.current!.querySelectorAll<HTMLElement>("[data-row]"))
      observer.current.observe(row);
    const anchored = (event: Event) => {
      const id = (event as CustomEvent<string>).detail;
      const row = latest.current.layout.rows.find((row) => row.id === id);
      if (!row) return;
      // A jump leaves a small gap above the target. Preserve that target's
      // position through measurement, instead of anchoring the row above it.
      jumpAnchor.current = { id, top: el.scrollTop };
      anchor.current = {
        id,
        offset: row.top + root.current!.offsetTop - el.scrollTop,
      };
    };
    const resetAnchor = () => {
      jumpAnchor.current = null;
    };
    el.addEventListener("reading-anchor", anchored);
    el.addEventListener("scroll-jump", resetAnchor);
    el.addEventListener("scroll", remember);
    el.addEventListener("reading-move", remember);
    remember();
    return () => {
      observer.current?.disconnect();
      el.removeEventListener("reading-anchor", anchored);
      el.removeEventListener("scroll-jump", resetAnchor);
      el.removeEventListener("scroll", remember);
      el.removeEventListener("reading-move", remember);
    };
  }, []);
  useLayoutEffect(() => {
    const el = pane.current;
    if (!el) return;
    const old = previous.current;
    const before = el.scrollTop;
    const oldMap = mapping.current;
    const beforeMapped = oldMap?.toLogical(before);
    if (old !== layout || (follow.current && !anchor.current)) {
      const row =
        anchor.current && layout.rows.find((r) => r.id === anchor.current!.id);
      if (follow.current) el.scrollTop = el.scrollHeight;
      else if (row)
        el.scrollTop =
          row.top + root.current!.offsetTop - anchor.current!.offset;
      if (jumpAnchor.current && row?.id === jumpAnchor.current.id)
        jumpAnchor.current.top = el.scrollTop;
      const delta = el.scrollTop - before;
      if (delta)
        el.dispatchEvent(new CustomEvent("history-shift", { detail: delta }));
      previous.current = layout;
    }
    const active = new Set(layout.rows.map((row) => row.id));
    for (const id of sizes.current.keys())
      if (!active.has(id)) sizes.current.delete(id);
    for (const id of knots.current.keys())
      if (!active.has(id)) knots.current.delete(id);
    // Preserve the reading fraction when its row is measured or reflows. A
    // monotonic piecewise map keeps both ends exact and dragging invertible.
    if (oldMap && beforeMapped !== undefined && !follow.current) {
      const index = rowAt(oldMap.rows, before - oldMap.origin);
      const oldRow = oldMap.rows[index];
      const row = oldRow && layout.rows.find((r) => r.id === oldRow.id);
      const fraction = (beforeMapped - oldMap.origin) / ROW_UNITS - index;
      const offset = row && el.scrollTop - root.current!.offsetTop - row.top;
      if (
        row &&
        offset! > 0 &&
        offset! < row.height &&
        fraction > 0 &&
        fraction < 1
      )
        knots.current.set(row.id, { offset: offset!, fraction });
    }
    const nextMap = messageMap(
      layout.rows,
      root.current!.offsetTop,
      knots.current,
      rowPadding,
    );
    mapping.current = nextMap;
    if (beforeMapped !== undefined) {
      // Rebase the rail by stable event coordinates, not a measured row's
      // fractional position. Rounding at a row boundary must not move ticks
      // against the reading direction when heights or the notice inset change.
      const delta = oldMap
        ? messageMapShift(oldMap, nextMap, before)
        : nextMap.toLogical(el.scrollTop) - beforeMapped;
      if (Math.abs(delta) > 0.001)
        el.dispatchEvent(new CustomEvent("mapping-shift", { detail: delta }));
    }
    changed(nextMap);
    remember();
  }, [layout, follow, rowPadding]);
  const { start, end } = visibleRows(
    layout.rows,
    viewport.top,
    viewport.height,
  );
  const latestActivity = useMemo(() => {
    for (let index = events.length - 1; index >= 0; index--)
      if (events[index].kind !== "input" && events[index].kind !== "assistant")
        return index;
    return -1;
  }, [events]);
  let promptIndex = -1;
  for (
    let i = 0;
    i < layout.rows.length && layout.rows[i].top + rowPadding < viewport.top;
    i++
  )
    if (layout.rows[i].prompt) promptIndex = i;
  const firstSeq = events[0]?.seq;
  const remembered = lastPrompt.current;
  const prompt =
    promptIndex >= 0
      ? events[promptIndex]
      : (precedingPrompt ??
        (remembered && firstSeq !== undefined && remembered.seq < firstSeq
          ? remembered
          : undefined));
  if (prompt) lastPrompt.current = prompt;
  // Apply the same clearance on both sides of the top edge: the next visible
  // input's top and the last passing input's bottom. Partial inputs leave no gap.
  const closest = layout.rows.find(
    (row) =>
      row.prompt &&
      row.top + row.height - rowPadding > viewport.offset &&
      row.top + rowPadding < viewport.offset + viewport.height,
  );
  const nextGap = closest
    ? Math.max(0, closest.top + rowPadding - viewport.offset)
    : Infinity;
  const previousRow = promptIndex >= 0 ? layout.rows[promptIndex] : undefined;
  const previousGap = previousRow
    ? Math.max(
        0,
        viewport.offset - (previousRow.top + previousRow.height - rowPadding),
      )
    : Infinity;
  const gap = Math.min(nextGap, previousGap);
  const progress = Math.max(0, Math.min(1, (gap - 96) / 32));
  const peek = progress * progress * (3 - 2 * progress);
  useLayoutEffect(() => {
    onPromptChange(prompt, peek);
  }, [prompt, peek, onPromptChange]);
  useTranscriptMotion({ root, pane, follow, rows: layout.rows, start, end });
  return (
    <div
      className="virtual-messages"
      ref={root}
      data-cached={events.length}
      data-first={events[0]?.seq.toString()}
      data-last={events.at(-1)?.seq.toString()}
      style={{ height: layout.total }}
    >
      {events.slice(start, end).map((event, index) => {
        const id = event.seq.toString();
        return (
          <div
            key={id}
            data-row={id}
            data-activity={event.kind !== "input" && event.kind !== "assistant"}
            data-latest-activity={start + index === latestActivity}
            className="transcript-row"
            style={{ top: layout.rows[start + index].top }}
            ref={(node) => {
              if (!node) return;
              observer.current?.observe(node);
              return () => {
                observer.current?.unobserve(node);
              };
            }}
          >
            <ActivityCardUnfolded value={start + index === latestActivity}>
              {render(event)}
            </ActivityCardUnfolded>
          </div>
        );
      })}
    </div>
  );
}
