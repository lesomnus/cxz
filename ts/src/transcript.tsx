import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  clamp,
  edgePull,
  scrollRange,
  markerRangeStart,
  type ScrollRange,
} from "./scroll-physics";

type Geometry = {
  top: number;
  max: number;
  viewport: number;
  height: number;
  thumb: number;
  range: ScrollRange;
  markers: { id: string; top: number }[];
};
const empty: Geometry = {
  top: 0,
  max: 0,
  viewport: 0,
  height: 0,
  thumb: 32,
  range: { start: 0, span: 0 },
  markers: [],
};
type Drag = {
  id: number;
  offset: number;
  pointer: number;
  range: ScrollRange;
  stretch: number;
  lastTime: number;
  edge: number;
};

export function Transcript({
  pane,
  children,
  onScroll,
  onTension,
  older,
  newer,
}: {
  pane: React.RefObject<HTMLDivElement | null>;
  children: React.ReactNode;
  onScroll: () => void;
  onTension: (stretch: number) => void;
  older: () => void;
  newer: () => void;
}) {
  const content = useRef<HTMLDivElement>(null);
  const track = useRef<HTMLDivElement>(null);
  const thumb = useRef<HTMLDivElement>(null);
  const offsets = useRef<{ id: string; y: number }[]>([]);
  const geometry = useRef(empty);
  const drag = useRef<Drag | null>(null);
  const raf = useRef(0);
  const [view, setView] = useState(empty);
  const [near, setNear] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [stretch, setStretch] = useState(0);
  const callbacks = useRef({ onScroll, onTension, older, newer });
  callbacks.current = { onScroll, onTension, older, newer };

  function measure(rebase = false) {
    const el = pane.current,
      rail = track.current;
    if (!el || !rail) return;
    const max = Math.max(0, el.scrollHeight - el.clientHeight);
    const range =
      drag.current?.range ??
      scrollRange(
        el.scrollTop,
        max,
        el.clientHeight,
        rebase ? undefined : geometry.current.range,
      );
    const height = rail.clientHeight;
    const size = clamp(
      (height * el.clientHeight) / (range.span + el.clientHeight || 1),
      32,
      height || 32,
    );
    // Stable event IDs preserve the same DOM nodes through range rebasing.
    // Keep off-rail ticks mounted so incoming/outgoing markers animate too.
    const markerStart = markerRangeStart(el.scrollTop, range);
    const markers = offsets.current.map(({ id, y }) => ({
      id,
      top: ((y - markerStart) / (range.span + el.clientHeight || 1)) * height,
    }));
    geometry.current = {
      top: el.scrollTop,
      max,
      viewport: el.clientHeight,
      height,
      thumb: size,
      range,
      markers,
    };
    setView(geometry.current);
  }
  function readOffsets() {
    const el = pane.current;
    if (!el) return;
    const top = el.getBoundingClientRect().top;
    offsets.current = [
      ...el.querySelectorAll<HTMLElement>("article.input"),
    ].map((node) => ({
      id: node.dataset.seq!,
      y: node.getBoundingClientRect().top - top + el.scrollTop,
    }));
    measure();
    callbacks.current.onScroll();
  }
  function scrolled() {
    measure();
    callbacks.current.onScroll();
    const el = pane.current;
    if (!el) return;
    if (el.scrollTop < el.clientHeight * 2) callbacks.current.older();
    if (el.scrollHeight - el.scrollTop - el.clientHeight < el.clientHeight * 2)
      callbacks.current.newer();
  }
  useLayoutEffect(readOffsets, [children]);
  useEffect(() => {
    const el = pane.current,
      inner = content.current;
    if (!el || !inner) return;
    const resize = new ResizeObserver(readOffsets);
    resize.observe(el);
    resize.observe(inner);
    const shifted = (event: Event) => {
      const delta = (event as CustomEvent<number>).detail;
      if (drag.current) drag.current.range.start += delta;
      else geometry.current.range.start += delta;
      readOffsets();
    };
    el.addEventListener("history-shift", shifted);
    return () => {
      resize.disconnect();
      el.removeEventListener("history-shift", shifted);
      cancelAnimationFrame(raf.current);
      drag.current = null;
    };
  }, []);

  function animate(time: number) {
    const d = drag.current,
      el = pane.current,
      rail = track.current;
    if (!d || !el || !rail) return;
    const g = geometry.current;
    const travel = Math.max(1, g.height - g.thumb);
    const raw = d.pointer - rail.getBoundingClientRect().top - d.offset;
    const bounded = clamp(raw, 0, travel);
    const outside = raw - bounded;
    const pull = edgePull(outside, g.viewport);
    d.stretch = pull.stretch;
    setStretch(pull.stretch);
    callbacks.current.onTension(pull.stretch);
    const dt = Math.min(32, time - d.lastTime) / 1000;
    d.lastTime = time;
    if (outside) {
      if (d.edge !== Math.sign(outside))
        el.scrollTop = d.range.start + (outside > 0 ? d.range.span : 0);
      el.scrollTop += pull.speed * dt;
    } else el.scrollTop = d.range.start + (bounded / travel) * d.range.span;
    d.edge = Math.sign(outside);
    scrolled();
    raf.current = requestAnimationFrame(animate);
  }
  function endDrag() {
    drag.current = null;
    cancelAnimationFrame(raf.current);
    setDragging(false);
    setNear(false);
    setStretch(0);
    callbacks.current.onTension(0);
    measure(true);
  }
  function keyScroll(e: React.KeyboardEvent) {
    const el = pane.current;
    if (!el) return;
    const step =
      e.key === "ArrowUp"
        ? -48
        : e.key === "ArrowDown"
          ? 48
          : e.key === "PageUp"
            ? -el.clientHeight
            : e.key === "PageDown"
              ? el.clientHeight
              : 0;
    if (step || e.key === "Home" || e.key === "End") {
      e.preventDefault();
      el.scrollTop =
        e.key === "Home"
          ? 0
          : e.key === "End"
            ? el.scrollHeight
            : el.scrollTop + step;
      scrolled();
    }
  }
  const position = view.range.span
    ? clamp((view.top - view.range.start) / view.range.span, 0, 1) *
      (view.height - view.thumb)
    : 0;
  return (
    <div
      className={`transcript-area ${near ? "scroll-near" : ""} ${dragging ? "scroll-dragging" : ""}`}
      onPointerMove={(e) => {
        if (!drag.current)
          setNear(
            e.clientX > e.currentTarget.getBoundingClientRect().right - 52,
          );
      }}
      onPointerLeave={() => {
        if (!drag.current) setNear(false);
      }}
    >
      <div
        className="transcript"
        ref={pane}
        id="conversation-transcript"
        tabIndex={0}
        aria-label="Conversation"
        onScroll={scrolled}
      >
        <div className="transcript-content" ref={content}>
          {children}
        </div>
      </div>
      <div className="scroll-track" ref={track} aria-hidden={view.max === 0}>
        <div className="scroll-markers">
          {view.markers.map(({ id, top }) => (
            <span
              key={id}
              className="scroll-marker"
              data-prompt={id}
              style={{ top: top + stretch }}
              aria-hidden="true"
            />
          ))}
        </div>
        <div
          ref={thumb}
          className="scroll-thumb"
          role="scrollbar"
          aria-label="Conversation scroll"
          aria-controls="conversation-transcript"
          aria-orientation="vertical"
          aria-valuemin={0}
          aria-valuemax={Math.round(view.max)}
          aria-valuenow={Math.round(view.top)}
          tabIndex={view.max > 0 ? 0 : -1}
          data-range={Math.round(view.range.span)}
          data-start={Math.round(view.range.start)}
          data-stretch={Math.abs(stretch).toFixed(2)}
          style={{
            top: position + stretch,
            height: view.thumb,
            visibility: view.max > 0 ? "visible" : "hidden",
          }}
          onKeyDown={keyScroll}
          onPointerDown={(e) => {
            if (e.button !== 0) return;
            e.preventDefault();
            e.currentTarget.setPointerCapture(e.pointerId);
            drag.current = {
              id: e.pointerId,
              offset: e.clientY - e.currentTarget.getBoundingClientRect().top,
              pointer: e.clientY,
              range: { ...geometry.current.range },
              stretch: 0,
              lastTime: performance.now(),
              edge: 0,
            };
            setDragging(true);
            callbacks.current.onScroll();
            raf.current = requestAnimationFrame(animate);
          }}
          onPointerMove={(e) => {
            if (drag.current?.id === e.pointerId)
              drag.current.pointer = e.clientY;
          }}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
          onLostPointerCapture={() => {
            if (drag.current) endDrag();
          }}
        >
          <span className="scroll-handle" />
        </div>
      </div>
    </div>
  );
}
