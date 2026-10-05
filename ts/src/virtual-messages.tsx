import React, {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import { messageLayout, rowAt, visibleRows } from "./virtual-layout";

export function VirtualMessages({
  pane,
  events,
  follow,
  render,
  changed,
}: {
  pane: React.RefObject<HTMLDivElement | null>;
  events: SessionEvent[];
  follow: boolean;
  render: (event: SessionEvent) => React.ReactNode;
  changed: (markers: { id: string; y: number }[]) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const sizes = useRef(new Map<string, number>());
  const expanded = useRef(new Map<string, boolean>());
  const [revision, setRevision] = useState(0);
  const [viewport, setViewport] = useState({ top: 0, height: 800 });
  const layout = useMemo(
    () => messageLayout(events, sizes.current),
    [events, revision],
  );
  const previous = useRef<typeof layout | null>(null);
  const latest = useRef({ layout, follow, changed });
  latest.current = { layout, follow, changed };
  const anchor = useRef<{ id: string; offset: number } | null>(null);
  const observer = useRef<ResizeObserver | null>(null);
  const width = useRef(0);

  function remember() {
    const el = pane.current;
    if (!el) return;
    const rows = latest.current.layout.rows;
    const row =
      rows[rowAt(rows, Math.max(0, el.scrollTop - root.current!.offsetTop))];
    anchor.current = row
      ? { id: row.id, offset: row.top + root.current!.offsetTop - el.scrollTop }
      : null;
    setViewport({
      top: Math.max(0, el.scrollTop - root.current!.offsetTop),
      height: el.clientHeight,
    });
  }
  useEffect(() => {
    const el = pane.current!;
    observer.current = new ResizeObserver((entries) => {
      let dirty = false;
      if (el.clientWidth !== width.current) {
        if (width.current) {
          sizes.current.clear();
          dirty = true;
        }
        width.current = el.clientWidth;
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
    el.addEventListener("scroll", remember);
    remember();
    return () => {
      observer.current?.disconnect();
      el.removeEventListener("scroll", remember);
    };
  }, []);
  useLayoutEffect(() => {
    const el = pane.current;
    if (!el) return;
    const old = previous.current;
    if (old !== layout || (follow && !anchor.current)) {
      const before = el.scrollTop;
      const row =
        anchor.current && layout.rows.find((r) => r.id === anchor.current!.id);
      if (follow) el.scrollTop = el.scrollHeight;
      else if (row)
        el.scrollTop =
          row.top + root.current!.offsetTop - anchor.current!.offset;
      const delta = el.scrollTop - before;
      if (delta)
        el.dispatchEvent(new CustomEvent("history-shift", { detail: delta }));
      previous.current = layout;
    }
    const active = new Set(layout.rows.map((row) => row.id));
    for (const id of sizes.current.keys())
      if (!active.has(id)) sizes.current.delete(id);
    for (const id of expanded.current.keys())
      if (!active.has(id)) expanded.current.delete(id);
    changed(
      layout.rows
        .filter((r) => r.prompt)
        .map((r) => ({ id: r.id, y: r.top + root.current!.offsetTop + 6 })),
    );
    remember();
  }, [layout, follow]);
  const { start, end } = visibleRows(
    layout.rows,
    viewport.top,
    viewport.height,
  );
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
            className="transcript-row"
            style={{ top: layout.rows[start + index].top }}
            ref={(node) => {
              if (!node) return;
              const detail = node.querySelector("details");
              if (detail && expanded.current.has(id))
                detail.open = expanded.current.get(id)!;
              const toggled = (e: Event) => {
                if (e.target instanceof HTMLDetailsElement)
                  expanded.current.set(id, e.target.open);
              };
              node.addEventListener("toggle", toggled, true);
              observer.current?.observe(node);
              return () => {
                observer.current?.unobserve(node);
                node.removeEventListener("toggle", toggled, true);
              };
            }}
          >
            {render(event)}
          </div>
        );
      })}
    </div>
  );
}
