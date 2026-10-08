import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { t } from "./i18n";

// Native scrolling keeps wheel, touch and focus navigation predictable; only
// its visual handle is custom, sharing the conversation handle's appearance.
export function PanelScroll({ children }: { children: ReactNode }) {
  const id = useId();
  const pane = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const rail = useRef<HTMLDivElement>(null);
  const drag = useRef<{ pointer: number; y: number; top: number } | null>(null);
  const [near, setNear] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [view, setView] = useState({ top: 0, max: 0, height: 0, thumb: 0 });

  function measure() {
    const el = pane.current;
    if (!el || !rail.current) return;
    const height = rail.current.clientHeight;
    const max = Math.max(0, el.scrollHeight - el.clientHeight);
    const thumb = Math.min(
      height,
      Math.max(32, (height * el.clientHeight) / (el.scrollHeight || 1)),
    );
    setView({
      top: Math.max(0, Math.min(max, el.scrollTop)),
      max,
      height,
      thumb,
    });
  }
  useEffect(() => {
    const observer = new ResizeObserver(measure);
    observer.observe(pane.current!);
    observer.observe(content.current!);
    observer.observe(rail.current!);
    measure();
    return () => observer.disconnect();
  }, []);

  const travel = Math.max(0, view.height - view.thumb);
  return (
    <div
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
        onScroll={measure}
      >
        <div ref={content} className="panel-scroll-inner">
          {children}
        </div>
      </div>
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
