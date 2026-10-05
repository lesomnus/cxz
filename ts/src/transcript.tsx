import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  clamp,
  edgePull,
  scrollRange,
  markerRangeStart,
  resumeRange,
  smoothStep,
  ELASTIC_RESERVE,
  type ScrollRange,
} from "./scroll-physics";

import type { SessionEvent } from "../gen/cxz/session_pb";
import { VirtualMessages } from "./virtual-messages";
import { Button } from "./button";
import type { MessageMap } from "./virtual-layout";

type Geometry = {
  top: number;
  mappedTop: number;
  max: number;
  viewport: number;
  height: number;
  thumb: number;
  range: ScrollRange;
  markers: { id: string; top: number }[];
};
const empty: Geometry = {
  top: 0,
  mappedTop: 0,
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
type RangeMotion = {
  time: number;
  thumb: number;
  markers: Map<string, number>;
  progress: number;
};

export function Transcript({
  pane,
  events,
  follow,
  render,
  notice,
  navigation,
  onScroll,
  onNavigate,
  onTension,
  onMotion,
  precedingPrompt,
  jumpTarget,
  onPromptJump,
  onJumped,
  older,
  newer,
}: {
  pane: React.RefObject<HTMLDivElement | null>;
  events: SessionEvent[];
  follow: React.RefObject<boolean>;
  render: (event: SessionEvent) => React.ReactNode;
  notice: React.ReactNode;
  navigation: React.ReactNode;
  onScroll: (reading: boolean) => void;
  onNavigate: () => void;
  onTension: (stretch: number) => void;
  onMotion: (motion: number) => void;
  precedingPrompt: SessionEvent | undefined;
  jumpTarget: string | undefined;
  onPromptJump: (seq: string) => void;
  onJumped: () => void;
  older: () => void;
  newer: () => void;
}) {
  const content = useRef<HTMLDivElement>(null);
  const pinnedButton = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState<{
    prompt: SessionEvent | undefined;
    peek: number;
  }>({ prompt: undefined, peek: 0 });
  const { prompt, peek } = pinned;
  const promptChanged = React.useCallback(
    (prompt: SessionEvent | undefined, peek: number) => {
      setPinned({ prompt, peek });
    },
    [],
  );
  const track = useRef<HTMLDivElement>(null);
  const thumb = useRef<HTMLDivElement>(null);
  const offsets = useRef<{ id: string; y: number }[]>([]);
  const mapping = useRef<MessageMap | null>(null);
  const geometry = useRef(empty);
  const drag = useRef<Drag | null>(null);
  const raf = useRef(0);
  const wheelRaf = useRef(0);
  const rangeRaf = useRef(0);
  const motionRaf = useRef(0);
  const motion = useRef({ top: 0, time: 0, strength: 0 });
  const rangeMotion = useRef<RangeMotion | null>(null);
  const [rangeFrame, setRangeFrame] = useState<RangeMotion | null>(null);
  const wheel = useRef<{ target: number; time: number; top: number } | null>(
    null,
  );
  const [view, setView] = useState(empty);
  const [near, setNear] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [stretch, setStretch] = useState(0);
  const callbacks = useRef({
    onScroll,
    onNavigate,
    onTension,
    onMotion,
    older,
    newer,
  });
  callbacks.current = {
    onScroll,
    onNavigate,
    onTension,
    onMotion,
    older,
    newer,
  };

  function jumpToPrompt(seq: string) {
    const el = pane.current;
    const map = mapping.current;
    const row = map?.rows.find((row) => row.id === seq);
    if (!el || !map) return;
    if (!row) {
      onPromptJump(seq);
      return;
    }
    el.dispatchEvent(new Event("scroll-jump"));
    callbacks.current.onNavigate();
    el.scrollTop = Math.max(0, map.origin + row.top + 6);
    scrolled();
    el.focus({ preventScroll: true });
  }
  useLayoutEffect(() => {
    if (
      jumpTarget &&
      mapping.current?.rows.some((row) => row.id === jumpTarget)
    ) {
      jumpToPrompt(jumpTarget);
      onJumped();
    }
  }, [jumpTarget, events]);

  function fadeMotion(time: number) {
    const sample = motion.current;
    const strength = sample.strength * Math.exp(-(time - sample.time) / 110);
    callbacks.current.onMotion(Math.abs(strength) < 0.01 ? 0 : strength);
    if (Math.abs(strength) >= 0.01)
      motionRaf.current = requestAnimationFrame(fadeMotion);
  }

  function stopRangeMotion() {
    cancelAnimationFrame(rangeRaf.current);
    rangeMotion.current = null;
    setRangeFrame(null);
  }
  function animateRange(time: number) {
    const motion = rangeMotion.current;
    if (!motion) return;
    const elapsed = clamp((time - motion.time) / 160, 0, 1);
    const next = { ...motion, progress: 1 - Math.pow(1 - elapsed, 3) };
    if (elapsed === 1) {
      rangeMotion.current = null;
      setRangeFrame(null);
    } else {
      setRangeFrame(next);
      rangeRaf.current = requestAnimationFrame(animateRange);
    }
  }
  function beginRangeMotion() {
    if (
      rangeMotion.current ||
      !thumb.current ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const motion = {
      time: performance.now(),
      progress: 0,
      thumb: parseFloat(getComputedStyle(thumb.current).top),
      markers: new Map(
        [...track.current!.querySelectorAll<HTMLElement>(".scroll-marker")].map(
          (node) => [
            node.dataset.prompt!,
            parseFloat(getComputedStyle(node).top),
          ],
        ),
      ),
    };
    rangeMotion.current = motion;
    setRangeFrame(motion);
    rangeRaf.current = requestAnimationFrame(animateRange);
  }

  function stopWheel() {
    cancelAnimationFrame(wheelRaf.current);
    wheel.current = null;
  }
  function animateWheel(time: number) {
    const el = pane.current,
      motion = wheel.current;
    if (!el || !motion) return;
    if (Math.abs(el.scrollTop - motion.top) > 1) {
      stopWheel();
      scrolled();
      return;
    }
    motion.target = clamp(motion.target, 0, el.scrollHeight - el.clientHeight);
    el.scrollTop = smoothStep(
      el.scrollTop,
      motion.target,
      Math.min(32, time - motion.time),
    );
    motion.time = time;
    motion.top = el.scrollTop;
    if (Math.abs(motion.top - motion.target) < 1) wheel.current = null;
    scrolled();
    if (wheel.current) wheelRaf.current = requestAnimationFrame(animateWheel);
  }

  function measure(rebase = false) {
    const el = pane.current,
      rail = track.current;
    if (!el || !rail) return;
    const max = Math.max(0, el.scrollHeight - el.clientHeight);
    const mappedTop = mapping.current?.toLogical(el.scrollTop) ?? el.scrollTop;
    const mappedMax = mapping.current?.toLogical(max) ?? max;
    const range =
      drag.current?.range ??
      scrollRange(
        mappedTop,
        mappedMax,
        el.clientHeight,
        rebase || follow.current ? undefined : geometry.current.range,
        geometry.current.mappedTop,
      );
    const height = rail.clientHeight;
    const size = clamp(
      (height * el.clientHeight) / (range.span + el.clientHeight || 1),
      32,
      height || 32,
    );
    // Stable event IDs preserve the same DOM nodes through range rebasing.
    // Keep off-rail ticks mounted so incoming/outgoing markers animate too.
    const markerStart = markerRangeStart(mappedTop, range);
    const markers = offsets.current.map(({ id, y }) => ({
      id,
      top: ((y - markerStart) / (range.span + el.clientHeight || 1)) * height,
    }));
    // One timeline for the handle and every marker. Height corrections retarget
    // that timeline instead of restarting an individual CSS transition.
    if (
      !drag.current &&
      !follow.current &&
      (rebase ||
        range.start !== geometry.current.range.start ||
        range.span !== geometry.current.range.span)
    )
      beginRangeMotion();
    geometry.current = {
      top: el.scrollTop,
      mappedTop,
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
    measure();
    callbacks.current.onScroll(!!(drag.current || wheel.current));
  }
  function scrolled() {
    const el = pane.current;
    if (!el) return;
    // Programmatic wheel/drag frames precede the browser's native scroll event.
    // Publish their anchor now so a ResizeObserver cannot restore a stale frame.
    el.dispatchEvent(new Event("reading-move"));
    const sample = motion.current;
    const delta = el.scrollTop - sample.top;
    if (delta && sample.time && !follow.current) {
      const time = performance.now();
      sample.strength = clamp(
        delta / clamp(time - sample.time, 16, 48) / 1.5,
        -1,
        1,
      );
      sample.time = time;
      callbacks.current.onMotion(sample.strength);
      cancelAnimationFrame(motionRaf.current);
      motionRaf.current = requestAnimationFrame(fadeMotion);
    } else if (!sample.time) sample.time = performance.now();
    sample.top = el.scrollTop;
    if (
      wheel.current &&
      Math.abs(pane.current!.scrollTop - wheel.current.top) > 1
    )
      stopWheel();
    measure();
    callbacks.current.onScroll(!!(drag.current || wheel.current));
    if (el.scrollTop < el.clientHeight * 2) callbacks.current.older();
    if (el.scrollHeight - el.scrollTop - el.clientHeight < el.clientHeight * 2)
      callbacks.current.newer();
  }
  useEffect(() => {
    const el = pane.current,
      inner = content.current;
    if (!el || !inner) return;
    const resize = new ResizeObserver(readOffsets);
    resize.observe(el);
    resize.observe(inner);
    const shifted = (event: Event) => {
      const delta = (event as CustomEvent<number>).detail;
      geometry.current.top += delta;
      motion.current.top += delta;
      if (wheel.current) {
        wheel.current.target += delta;
        wheel.current.top += delta;
      }
      // VirtualMessages publishes the matching prompt offsets immediately after
      // this event; measuring now would mix new coordinates with old markers.
    };
    const mappedShift = (event: Event) => {
      const delta = (event as CustomEvent<number>).detail;
      geometry.current.mappedTop += delta;
      if (drag.current) drag.current.range.start += delta;
      else geometry.current.range.start += delta;
    };
    const wheeled = (event: WheelEvent) => {
      if (
        event.ctrlKey ||
        event.metaKey ||
        drag.current ||
        !event.deltaY ||
        Math.abs(event.deltaX) > Math.abs(event.deltaY)
      )
        return;
      // Preserve native scrolling of nested code blocks, and OS trackpad motion.
      for (
        let node = event.target as HTMLElement | null;
        node && node !== el;
        node = node.parentElement
      ) {
        if (
          node.scrollHeight > node.clientHeight + 1 &&
          /auto|scroll/.test(getComputedStyle(node).overflowY) &&
          (event.deltaY < 0
            ? node.scrollTop > 0
            : node.scrollTop + node.clientHeight < node.scrollHeight)
        )
          return;
      }
      if (
        window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
        (event.deltaMode === 0 && Math.abs(event.deltaY) < 40)
      ) {
        stopWheel();
        if (
          event.deltaY < 0
            ? el.scrollTop > 0
            : el.scrollTop < el.scrollHeight - el.clientHeight
        )
          callbacks.current.onNavigate();
        return;
      }
      event.preventDefault();
      const delta =
        event.deltaY *
        (event.deltaMode === 1
          ? 16
          : event.deltaMode === 2
            ? el.clientHeight
            : 1);
      const target = clamp(
        (wheel.current?.target ?? el.scrollTop) + delta,
        0,
        el.scrollHeight - el.clientHeight,
      );
      if (target === (wheel.current?.target ?? el.scrollTop)) return;
      callbacks.current.onNavigate();
      const running = !!wheel.current;
      wheel.current = {
        target,
        time: wheel.current?.time ?? performance.now(),
        top: el.scrollTop,
      };
      if (!running) wheelRaf.current = requestAnimationFrame(animateWheel);
    };
    const interrupted = () => stopWheel();
    const jumped = () => {
      stopWheel();
      cancelAnimationFrame(motionRaf.current);
      motion.current = { top: el.scrollTop, time: 0, strength: 0 };
      callbacks.current.onMotion(0);
    };
    el.addEventListener("history-shift", shifted);
    el.addEventListener("mapping-shift", mappedShift);
    el.addEventListener("wheel", wheeled, { passive: false });
    el.addEventListener("pointerdown", interrupted);
    el.addEventListener("keydown", interrupted);
    el.addEventListener("scroll-jump", jumped);
    return () => {
      resize.disconnect();
      el.removeEventListener("history-shift", shifted);
      el.removeEventListener("mapping-shift", mappedShift);
      el.removeEventListener("wheel", wheeled);
      el.removeEventListener("pointerdown", interrupted);
      el.removeEventListener("keydown", interrupted);
      el.removeEventListener("scroll-jump", jumped);
      stopWheel();
      cancelAnimationFrame(rangeRaf.current);
      cancelAnimationFrame(motionRaf.current);
      rangeMotion.current = null;
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
        el.scrollTop =
          mapping.current?.toNative(
            d.range.start + (outside > 0 ? d.range.span : 0),
          ) ?? 0;
      el.scrollTop += pull.speed * dt;
    } else {
      if (d.edge)
        d.range = resumeRange(
          mapping.current?.toLogical(el.scrollTop) ?? el.scrollTop,
          bounded / travel,
          d.range.span,
        );
      const target = clamp(
        mapping.current?.toNative(
          d.range.start + (bounded / travel) * d.range.span,
        ) ?? 0,
        0,
        g.max,
      );
      el.scrollTop = window.matchMedia("(prefers-reduced-motion: reduce)")
        .matches
        ? target
        : smoothStep(el.scrollTop, target, dt * 1000, 12);
    }
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
    callbacks.current.onScroll(false);
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
      stopWheel();
      callbacks.current.onNavigate();
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
    ? clamp((view.mappedTop - view.range.start) / view.range.span, 0, 1) *
      (view.height - view.thumb)
    : 0;
  const animated = (target: number, from: number | undefined) =>
    rangeFrame && from !== undefined
      ? from + (target - from) * rangeFrame.progress
      : target;
  return (
    <div
      className={`transcript-area ${follow.current ? "scroll-following" : ""} ${near ? "scroll-near" : ""} ${dragging ? "scroll-dragging" : ""}`}
      style={
        { "--elastic-reserve": `${ELASTIC_RESERVE}px` } as React.CSSProperties
      }
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
          {notice}
          <VirtualMessages
            pane={pane}
            events={events}
            follow={follow}
            render={render}
            precedingPrompt={precedingPrompt}
            onPromptChange={promptChanged}
            changed={(nextMap) => {
              mapping.current = nextMap;
              offsets.current = nextMap.markers;
              readOffsets();
            }}
          />
        </div>
      </div>
      <div className="transcript-fade transcript-fade-top" aria-hidden="true" />
      <div
        className="transcript-fade transcript-fade-bottom"
        aria-hidden="true"
      />
      {prompt && (
        <div
          className="pinned-prompt"
          ref={pinnedButton}
          data-pinned-seq={prompt.seq.toString()}
          data-available={peek > 0}
          inert={peek === 0}
          aria-hidden={peek === 0}
          style={
            {
              "--prompt-peek": `${8 * peek}px`,
              opacity: peek,
              "--prompt-height": `${Math.min(180, (view.viewport || 720) * 0.25)}px`,
            } as React.CSSProperties
          }
        >
          <div
            className="pinned-prompt-proximity"
            aria-hidden="true"
            onPointerDown={() =>
              pinnedButton.current
                ?.querySelector("button")
                ?.focus({ preventScroll: true })
            }
          />
          <Button
            type="button"
            aria-label="Jump to user message"
            onClick={() => jumpToPrompt(prompt.seq.toString())}
          >
            <small>❯ You</small>
            <span className="message-body">{prompt.text}</span>
          </Button>
        </div>
      )}
      {navigation}
      <div className="scroll-track" ref={track} aria-hidden={view.max === 0}>
        <div className="scroll-markers">
          {view.markers.map(({ id, top }) => (
            <span
              key={id}
              className="scroll-marker"
              data-prompt={id}
              data-target-top={top + stretch}
              style={{
                top: animated(top + stretch, rangeFrame?.markers.get(id)),
              }}
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
          data-start={Math.round(
            mapping.current?.toNative(view.range.start) ?? view.range.start,
          )}
          data-stretch={Math.abs(stretch).toFixed(2)}
          data-target-top={position + stretch}
          style={{
            top: animated(position + stretch, rangeFrame?.thumb),
            height: view.thumb,
            visibility: view.max > 0 ? "visible" : "hidden",
          }}
          onKeyDown={keyScroll}
          onPointerDown={(e) => {
            if (e.button !== 0) return;
            e.preventDefault();
            stopWheel();
            callbacks.current.onNavigate();
            e.currentTarget.setPointerCapture(e.pointerId);
            const bounds = e.currentTarget.getBoundingClientRect();
            stopRangeMotion();
            const rail = track.current!.getBoundingClientRect();
            const fraction = clamp(
              (bounds.top - rail.top) /
                Math.max(1, geometry.current.height - geometry.current.thumb),
              0,
              1,
            );
            drag.current = {
              id: e.pointerId,
              offset: e.clientY - bounds.top,
              pointer: e.clientY,
              range: resumeRange(
                mapping.current?.toLogical(pane.current!.scrollTop) ??
                  pane.current!.scrollTop,
                fraction,
                geometry.current.range.span,
              ),
              stretch: 0,
              lastTime: performance.now(),
              edge: 0,
            };
            setDragging(true);
            measure();
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
