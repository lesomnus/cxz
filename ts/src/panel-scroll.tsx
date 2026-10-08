import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { t } from "./i18n";
import { clamp } from "./scroll-physics";

// Native scrolling keeps wheel, touch and focus navigation predictable; only
// its visual handle is custom, sharing the conversation handle's appearance.
export function PanelScroll({ children }: { children: ReactNode }) {
  const id = useId();
  const region = useRef<HTMLDivElement>(null);
  const pane = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const rail = useRef<HTMLDivElement>(null);
  const drag = useRef<{ pointer: number; y: number; top: number } | null>(null);
  const motion = useRef({ top: 0, time: 0, strength: 0 });
  const motionRaf = useRef(0);
  const [near, setNear] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [view, setView] = useState({ top: 0, max: 0, height: 0, thumb: 0 });

  function measure() {
    const el = pane.current;
    if (!el || !rail.current) return;
    const height = rail.current.clientHeight;
    const max = Math.max(0, el.scrollHeight - el.clientHeight);
    const top = clamp(el.scrollTop, 0, max);
    region.current?.style.setProperty("--panel-top-hidden", `${top}px`);
    region.current?.style.setProperty(
      "--panel-bottom-hidden",
      `${max - top}px`,
    );
    region.current?.style.setProperty(
      "--panel-top-opacity",
      top > 0 ? "1" : "0",
    );
    region.current?.style.setProperty(
      "--panel-bottom-opacity",
      max > top ? "1" : "0",
    );
    const thumb = Math.min(
      height,
      Math.max(32, (height * el.clientHeight) / (el.scrollHeight || 1)),
    );
    setView({
      top,
      max,
      height,
      thumb,
    });
  }
  function paintMotion(strength: number) {
    // Extend the edge where content leaves the viewport, without re-rendering
    // the session list on every animation frame.
    region.current?.style.setProperty(
      "--panel-top-motion",
      `${Math.max(0, strength)}`,
    );
    region.current?.style.setProperty(
      "--panel-bottom-motion",
      `${Math.max(0, -strength)}`,
    );
  }
  function settleMotion(time: number) {
    const sample = motion.current;
    const strength = sample.strength * Math.exp(-(time - sample.time) / 110);
    paintMotion(Math.abs(strength) < 0.01 ? 0 : strength);
    if (Math.abs(strength) >= 0.01)
      motionRaf.current = requestAnimationFrame(settleMotion);
  }
  function scrolled() {
    const el = pane.current;
    if (!el) return;
    const sample = motion.current;
    const time = performance.now();
    const delta = el.scrollTop - sample.top;
    if (delta) {
      sample.strength = clamp(
        delta / clamp(time - sample.time, 16, 48) / 1.5,
        -1,
        1,
      );
      sample.time = time;
      paintMotion(sample.strength);
      cancelAnimationFrame(motionRaf.current);
      motionRaf.current = requestAnimationFrame(settleMotion);
    }
    sample.top = el.scrollTop;
    measure();
  }
  useEffect(() => {
    const observer = new ResizeObserver(measure);
    observer.observe(pane.current!);
    observer.observe(content.current!);
    observer.observe(rail.current!);
    measure();
    return () => {
      observer.disconnect();
      cancelAnimationFrame(motionRaf.current);
    };
  }, []);

  const travel = Math.max(0, view.height - view.thumb);
  return (
    <div
      ref={region}
      className={`panel-scroll-region${near ? " scroll-near" : ""}${dragging ? " scroll-dragging" : ""}`}
      onPointerMove={(event) => {
        const bounds = event.currentTarget.getBoundingClientRect();
        setNear(event.clientX > bounds.right - 32);
      }}
      onPointerLeave={() => setNear(false)}
    >
      <div
        ref={pane}
        id={id}
        className="panel-scroll-content"
        onScroll={scrolled}
      >
        <div ref={content} className="panel-scroll-inner">
          {children}
        </div>
      </div>
      <div
        className="panel-scroll-fade panel-scroll-fade-top"
        aria-hidden="true"
      />
      <div
        className="panel-scroll-fade panel-scroll-fade-bottom"
        aria-hidden="true"
      />
      <div ref={rail} className="panel-scroll-rail">
        {view.max > 0 && (
          <div
            className="panel-scroll-thumb"
            role="scrollbar"
            tabIndex={0}
            aria-label={t("Panel scroll")}
            aria-orientation="vertical"
            aria-controls={id}
            aria-valuemin={0}
            aria-valuemax={Math.round(view.max)}
            aria-valuenow={Math.round(view.top)}
            style={{
              top: view.max ? (view.top / view.max) * travel : 0,
              height: view.thumb,
            }}
            onPointerDown={(event) => {
              if (event.button !== 0 || !travel) return;
              event.preventDefault();
              event.currentTarget.focus({ preventScroll: true });
              event.currentTarget.setPointerCapture(event.pointerId);
              drag.current = {
                pointer: event.pointerId,
                y: event.clientY,
                top: pane.current!.scrollTop,
              };
              setDragging(true);
            }}
            onPointerMove={(event) => {
              const active = drag.current;
              if (!active || active.pointer !== event.pointerId || !travel)
                return;
              pane.current!.scrollTop =
                active.top + ((event.clientY - active.y) * view.max) / travel;
            }}
            onLostPointerCapture={() => {
              drag.current = null;
              setDragging(false);
            }}
            onKeyDown={(event) => {
              const el = pane.current!;
              const positions: Record<string, number> = {
                ArrowUp: el.scrollTop - 40,
                ArrowDown: el.scrollTop + 40,
                PageUp: el.scrollTop - el.clientHeight,
                PageDown: el.scrollTop + el.clientHeight,
                Home: 0,
                End: view.max,
              };
              const next = positions[event.key];
              if (next === undefined) return;
              event.preventDefault();
              el.scrollTop = next;
            }}
          >
            <span className="scroll-handle" />
          </div>
        )}
      </div>
    </div>
  );
}
