import {
  eventListSource,
  snapshotSource,
  type EventListSource,
} from "#src/features/session/model/event-source.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
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

import type { SessionEvent } from "#gen/cxz/session_pb";
import { VirtualMessages, type TranscriptViewport } from "./virtual-messages";
import { PinnedPrompt } from "./pinned-prompt";
import type { MessageMap } from "./virtual-layout";
import { HISTORY_PREFETCH_SCREENS } from "#src/features/session/model/session-history.ts";
import {
  workGroups,
  selectedWorkGroup,
} from "#src/features/session/model/work-groups.ts";
import { WorkDialog, WorkSummary } from "./work-dialog";
import { ScrollMarkers, type ScrollTick } from "./scroll-markers";
import {
  scrollMarkerWindow,
  type MarkerCoordinate,
} from "./scroll-marker-window";

type Geometry = {
  top: number;
  mappedTop: number;
  max: number;
  viewport: number;
  height: number;
  thumb: number;
  range: ScrollRange;
  markers: ScrollTick[];
};
export type WorkHistory = { hasOlder: boolean; loadOlder: () => Promise<void> };
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
  source,
  follow,
  render,
  notice,
  loading,
  onScroll,
  onNavigate,
  onTension,
  onMotion,
  onReadingMove,
  precedingPrompt,
  jumpTarget,
  jumpInset = 18,
  onPromptJump,
  onJumped,
  older,
  newer,
  workHistory,
  viewportCache,
  searchMatches,
}: {
  pane: React.RefObject<HTMLDivElement | null>;
  source: EventListSource;
  follow: React.RefObject<boolean>;
  render: (event: SessionEvent) => React.ReactNode;
  notice: React.ReactNode;
  loading?: React.ReactNode;
  onScroll: (reading: boolean) => void;
  onNavigate: () => void;
  onTension: (stretch: number) => void;
  onMotion: (motion: number) => void;
  onReadingMove: (delta: number) => void;
  precedingPrompt: SessionEvent | undefined;
  jumpTarget: string | undefined;
  jumpInset?: number;
  onPromptJump: (seq: string) => void;
  onJumped: () => void;
  older: () => void;
  newer: () => void;
  workHistory?: WorkHistory;
  viewportCache?: TranscriptViewport;
  searchMatches?: readonly string[];
}) {
  const events = source();
  useLocale();
  const work = React.useMemo(() => workGroups(events), [events]);
  const [selectedWork, selectWork] = useState<SessionEvent>();
  const [workClosing, setWorkClosing] = useState(false);
  const restoreWorkFocus = useRef(true);
  const activeWork = selectedWorkGroup(work.groups, selectedWork);
  const workTrigger = useRef<HTMLButtonElement | null>(null);
  const closeWork = React.useCallback((restore = true) => {
    restoreWorkFocus.current = restore;
    setWorkClosing(true);
  }, []);
  const renderRow = React.useCallback(
    (event: SessionEvent) => {
      const group = work.summaries.get(event.seq);
      return group ? (
        <WorkSummary
          id={group.id}
          count={group.events.length}
          durationMs={group.durationMs}
          expanded={activeWork === group && !workClosing}
          partial={!group.hasBoundary && !!workHistory?.hasOlder}
          open={(trigger) => {
            workTrigger.current = trigger;
            if (activeWork === group && !workClosing) closeWork();
            else {
              setWorkClosing(false);
              selectWork(group.anchor);
            }
          }}
        />
      ) : (
        render(event)
      );
    },
    [work, activeWork, workClosing, workHistory?.hasOlder, closeWork, render],
  );
  useEffect(() => {
    if (!workClosing) return;
    const node = document.getElementById("work-dialog");
    const token =
      node &&
      getComputedStyle(node).getPropertyValue("--ui-dialog-enter-duration");
    const duration = token
      ? parseFloat(token) * (token.trim().endsWith("ms") ? 1 : 1000)
      : 0;
    const timer = setTimeout(
      () => {
        selectWork(undefined);
        setWorkClosing(false);
      },
      matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : duration,
    );
    return () => clearTimeout(timer);
  }, [workClosing, pane]);
  useEffect(() => {
    if (selectedWork && !activeWork) selectWork(undefined);
  }, [selectedWork, activeWork]);
  const content = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState<{
    prompt: SessionEvent | undefined;
    peek: number;
  }>({ prompt: undefined, peek: 0 });
  const { prompt, peek } = pinned;
  const promptChanged = React.useCallback(
    (prompt: SessionEvent | undefined, peek: number) => {
      setPinned((old) =>
        old.prompt === prompt && old.peek === peek ? old : { prompt, peek },
      );
    },
    [],
  );
  const track = useRef<HTMLDivElement>(null);
  const thumb = useRef<HTMLDivElement>(null);
  const markerNodes = useRef(new Map<string, HTMLSpanElement>());
  const offsets = useRef<MessageMap["markers"]>([]);
  const markerCoordinates = useRef<MarkerCoordinate[]>([]);
  const markersById = useRef(new Map<string, MarkerCoordinate>());
  const indexedMarkers = useRef<
    | { offsets: MessageMap["markers"]; searches: ReadonlySet<string> }
    | undefined
  >(undefined);
  const searchIds = useRef<ReadonlySet<string>>(new Set());
  searchIds.current = React.useMemo(
    () => new Set(searchMatches),
    [searchMatches],
  );
  const mapping = useRef<MessageMap | null>(null);
  const geometry = useRef(empty);
  const drag = useRef<Drag | null>(null);
  const raf = useRef(0);
  const wheelRaf = useRef(0);
  const rangeRaf = useRef(0);
  const motionRaf = useRef(0);
  const fillRaf = useRef(0);
  const readingDirection = useRef(0);
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
    onReadingMove,
    older,
    newer,
  });
  callbacks.current = {
    onScroll,
    onNavigate,
    onTension,
    onMotion,
    onReadingMove,
    older,
    newer,
  };

  function jumpToPrompt(seq: string, inset = 18) {
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
    el.scrollTop = Math.max(0, map.origin + row.top + map.rowPadding - inset);
    scrolled();
    el.dispatchEvent(new CustomEvent("reading-anchor", { detail: seq }));
    el.focus({ preventScroll: true });
  }
  useLayoutEffect(() => {
    if (
      jumpTarget &&
      mapping.current?.rows.some((row) => row.id === jumpTarget)
    ) {
      jumpToPrompt(jumpTarget, jumpInset);
      onJumped();
    }
  }, [jumpTarget, jumpInset, events]);

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
    const railHeight = track.current!.clientHeight;
    const motion = {
      time: performance.now(),
      progress: 0,
      // Ordinary handle motion can still be easing toward its target. Rebase
      // from the painted position so switching to the shared timeline is smooth.
      thumb: parseFloat(getComputedStyle(thumb.current).top),
      markers: new Map(
        [...markerNodes.current.values()]
          .map((node): [string, number] => [
            node.dataset.event!,
            parseFloat(node.style.top),
          ])
          .filter(([, top]) => top >= -railHeight && top <= railHeight * 2),
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
  function rebaseWheel(el: HTMLDivElement) {
    const motion = wheel.current;
    if (!motion) return;
    const top = clamp(motion.top, 0, el.scrollHeight - el.clientHeight);
    if (top === motion.top || Math.abs(el.scrollTop - top) > 1) return;
    // Row measurements or viewport growth can clamp a bottom position before
    // the first wheel frame. Preserve its remaining travel through that clamp;
    // it is a layout correction, not another gesture interrupting the wheel.
    motion.target += top - motion.top;
    motion.top = top;
  }
  function animateWheel(time: number) {
    const el = pane.current,
      motion = wheel.current;
    if (!el || !motion) return;
    rebaseWheel(el);
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
    const span = range.span + el.clientHeight || 1;
    const markers = scrollMarkerWindow(
      markerCoordinates.current,
      markersById.current,
      markerStart - span,
      markerStart + 2 * span,
      rangeMotion.current?.markers,
    ).map(({ id, kind, y }) => ({
      id,
      kind,
      top: ((y - markerStart) / span) * height,
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
  useLayoutEffect(() => {
    // A query changes ticks, not the scroll position or loaded history. Reuse
    // the same range/tension geometry and animation timeline as existing ticks.
    indexMarkers();
    measure();
  }, [searchMatches]);
  function indexMarkers() {
    if (
      indexedMarkers.current?.offsets === offsets.current &&
      indexedMarkers.current.searches === searchIds.current
    )
      return;
    indexedMarkers.current = {
      offsets: offsets.current,
      searches: searchIds.current,
    };
    markerCoordinates.current = offsets.current.flatMap(
      (marker): MarkerCoordinate[] =>
        searchIds.current.has(marker.id)
          ? [marker, { ...marker, id: `search:${marker.id}`, kind: "search" }]
          : [marker],
    );
    markersById.current = new Map(
      markerCoordinates.current.map((marker) => [marker.id, marker]),
    );
  }
  const offsetsChanged = React.useCallback(
    (nextMap: MessageMap) => {
      mapping.current = nextMap;
      offsets.current = nextMap.markers;
      indexMarkers();
      readOffsets();
    },
    [pane, follow],
  );
  useLayoutEffect(() => {
    for (const { id, kind, top } of view.markers) {
      const node = markerNodes.current.get(id);
      if (!node) continue;
      const target = top + stretch;
      const before =
        rangeFrame?.markers.get(id) ??
        (kind === "search"
          ? rangeFrame?.markers.get(id.slice("search:".length))
          : undefined);
      const position = animated(target, before);
      node.style.top = `${position}px`;
      node.dataset.targetTop = String(target);
    }
  }, [view, stretch, rangeFrame]);
  function readOffsets() {
    const el = pane.current;
    if (!el) return;
    // A measurement/viewport change is not a reading gesture. Keep a follower
    // at the new bottom before publishing geometry, rather than detaching it
    // because the measured content temporarily grew below its old position.
    if (follow.current && !drag.current && !wheel.current)
      el.scrollTop = el.scrollHeight;
    measure();
    callbacks.current.onScroll(!!(drag.current || wheel.current));
    // Short/filtered pages cannot emit a native scroll event. Recheck after
    // layout/measurement and resize, not only after the user moves the pane.
    cancelAnimationFrame(fillRaf.current);
    fillRaf.current = requestAnimationFrame(() => {
      const max = el.scrollHeight - el.clientHeight;
      if (max < el.clientHeight * HISTORY_PREFETCH_SCREENS) {
        if (!follow.current && readingDirection.current > 0)
          callbacks.current.newer();
        else callbacks.current.older();
      } else if (
        !follow.current &&
        readingDirection.current < 0 &&
        el.scrollTop < el.clientHeight * HISTORY_PREFETCH_SCREENS
      )
        callbacks.current.older();
      else if (
        !follow.current &&
        readingDirection.current > 0 &&
        max - el.scrollTop < el.clientHeight * HISTORY_PREFETCH_SCREENS
      )
        callbacks.current.newer();
    });
  }
  function scrolled() {
    const el = pane.current;
    if (!el) return;
    rebaseWheel(el);
    // Native scroll events can precede ResizeObserver while the composer is
    // contracting. Correct a follower against the current layout before its
    // old scroll position is mistaken for a reading gesture. Ordinary native
    // navigation with unchanged dimensions still detaches as before.
    const previous = geometry.current;
    if (
      follow.current &&
      !drag.current &&
      !wheel.current &&
      (el.clientHeight !== previous.viewport ||
        Math.max(0, el.scrollHeight - el.clientHeight) !== previous.max)
    )
      el.scrollTop = el.scrollHeight;
    // Programmatic wheel/drag frames precede the browser's native scroll event.
    // Publish their anchor now so a ResizeObserver cannot restore a stale frame.
    el.dispatchEvent(new Event("reading-move"));
    const sample = motion.current;
    const delta = el.scrollTop - sample.top;
    if (delta && !follow.current) readingDirection.current = Math.sign(delta);
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
    if (delta) {
      callbacks.current.onReadingMove(delta);
      if (!follow.current)
        el.dispatchEvent(
          new CustomEvent("conversation-reading-move", { bubbles: true }),
        );
    }
    // Only request the edge being approached. A small loaded window can be
    // near both ends: fetching both would alternately evict opposite pages.
    if (delta < 0 && el.scrollTop < el.clientHeight * HISTORY_PREFETCH_SCREENS)
      callbacks.current.older();
    if (
      delta > 0 &&
      el.scrollHeight - el.scrollTop - el.clientHeight <
        el.clientHeight * HISTORY_PREFETCH_SCREENS
    )
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
        else {
          readingDirection.current = Math.sign(event.deltaY);
          if (event.deltaY < 0) callbacks.current.older();
          else callbacks.current.newer();
        }
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
      if (target === (wheel.current?.target ?? el.scrollTop)) {
        // At a loaded edge there may still be retained history beyond it.
        // Fetching does not mean navigation until actual content can move.
        readingDirection.current = Math.sign(delta);
        if (delta < 0) callbacks.current.older();
        else callbacks.current.newer();
        return;
      }
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
      cancelAnimationFrame(fillRaf.current);
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
      cancelAnimationFrame(fillRaf.current);
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
      className={`transcript-area ${follow.current ? "scroll-following" : ""} ${near ? "scroll-near" : ""} ${dragging ? "scroll-dragging" : ""} ${rangeFrame ? "scroll-rebasing" : ""}`}
      onPointerDownCapture={(event) => {
        if (
          activeWork &&
          event.target instanceof Element &&
          !event.target.closest("#work-dialog, [data-work-summary]")
        )
          closeWork(false);
      }}
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
        aria-label={t("Conversation")}
        aria-busy={Boolean(loading)}
        onScroll={scrolled}
      >
        <div className="transcript-content" ref={content}>
          {notice}
          <VirtualMessages
            cache={viewportCache}
            pane={pane}
            source={eventListSource(work.events)}
            follow={follow}
            foldActivities={false}
            render={renderRow}
            precedingPrompt={precedingPrompt}
            onPromptChange={promptChanged}
            changed={offsetsChanged}
          />
        </div>
      </div>
      {loading}
      {activeWork && (
        <WorkDialog
          key={`${selectedWork!.runId}/${selectedWork!.requestId || selectedWork!.seq}`}
          source={snapshotSource(activeWork)}
          render={render}
          close={closeWork}
          closing={workClosing}
          history={workHistory}
          container={pane.current!.closest<HTMLElement>(".conversation")!}
          onClosed={() => {
            if (restoreWorkFocus.current)
              (workTrigger.current?.isConnected
                ? workTrigger.current
                : pane.current
              )?.focus({ preventScroll: true });
          }}
        />
      )}
      <div className="transcript-fade transcript-fade-top" aria-hidden="true" />
      <div
        className="transcript-fade transcript-fade-bottom"
        aria-hidden="true"
      />
      {prompt && (
        <PinnedPrompt
          prompt={prompt}
          peek={peek}
          height={Math.min(180, (view.viewport || 720) * 0.25)}
          jump={(seq) => {
            jumpToPrompt(seq);
            pane.current?.focus({ preventScroll: true });
          }}
        />
      )}
      <div className="scroll-track" ref={track} aria-hidden={view.max === 0}>
        <ScrollMarkers markers={view.markers} nodes={markerNodes} />
        <div
          ref={thumb}
          className="scroll-thumb"
          role="scrollbar"
          aria-label={t("Conversation scroll")}
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
