import {
  createContext,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "@lesomnus/cxz-ui";
import { EventTimePopover } from "./event-time-popover";
import { useAnchoredCard } from "../cards/floating-card";

export const ActivityCardUnfolded = createContext(false);

// The virtual row owns the folded footprint; this face can unfold above its
// neighbors without changing the transcript's measured heights or scroll map.
export function ActivityCard({
  seq,
  timeMs,
  title,
  children,
  details: content,
  className = "",
  state,
}: {
  seq: bigint;
  timeMs: bigint;
  title: string;
  children: ReactNode;
  details: () => ReactNode;
  className?: string;
  state?: string;
}) {
  const unfolded = useContext(ActivityCardUnfolded);
  const surface = useRef<HTMLDivElement>(null);
  const details = useAnchoredCard(surface);
  const timestamp = useId();
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [pressed, setPressed] = useState(false);
  const active = hovered || focused || details.present;
  const [present, setPresent] = useState(false);
  const [entered, setEntered] = useState(false);
  const [area, setArea] = useState<HTMLElement | null>(null);
  useLayoutEffect(() => {
    setArea(
      details.anchor.current?.closest<HTMLElement>(".transcript-area") ?? null,
    );
  }, []);
  useEffect(() => {
    if (!pressed) return;
    const release = () => setPressed(false);
    window.addEventListener("pointerup", release);
    window.addEventListener("pointercancel", release);
    return () => {
      window.removeEventListener("pointerup", release);
      window.removeEventListener("pointercancel", release);
    };
  }, [pressed]);
  useEffect(() => {
    if (!active && !present) return;
    let frame = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    if (active) {
      setPresent(true);
      frame = requestAnimationFrame(() => setEntered(true));
    } else {
      setEntered(false);
      const token =
        details.anchor.current &&
        getComputedStyle(details.anchor.current).getPropertyValue(
          "--activity-card-duration",
        );
      const duration = token
        ? parseFloat(token) * (token.trim().endsWith("ms") ? 1 : 1000)
        : 0;
      timer = setTimeout(() => setPresent(false), duration);
    }
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
    };
  }, [active]);
  useLayoutEffect(() => {
    const anchor = details.anchor.current;
    const preview = surface.current;
    const row = anchor?.closest(".transcript-row");
    const virtual = anchor?.closest(".virtual-messages");
    const pane = area?.querySelector(".transcript");
    if (!anchor || !preview || !row || !virtual || !area || !pane) return;
    let frame = 0;
    const measure = () => {
      frame = 0;
      const origin = anchor.getBoundingClientRect();
      const bounds = area.getBoundingClientRect();
      preview.style.left = `${origin.left - bounds.left}px`;
      preview.style.top = `${origin.top - bounds.top}px`;
      preview.style.width = `${origin.width}px`;
      preview.style.setProperty(
        "--press-scale",
        getComputedStyle(anchor.firstElementChild!).getPropertyValue(
          "--press-scale",
        ),
      );
      // Inside the clipped virtual root the original button receives input.
      // Only its overflow needs a portal hit target, preserving native focus,
      // pointer activation and a single accessible button for each activity.
      preview.style.setProperty(
        "--activity-hit-start",
        `${Math.max(0, virtual.getBoundingClientRect().bottom - origin.top)}px`,
      );
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    measure();
    const resize = new ResizeObserver(schedule);
    resize.observe(anchor);
    resize.observe(area);
    const mutations = new MutationObserver(schedule);
    mutations.observe(row, { attributes: true, attributeFilter: ["style"] });
    mutations.observe(virtual, { childList: true });
    for (const event of ["scroll", "history-shift", "mapping-shift"])
      pane.addEventListener(event, schedule);
    return () => {
      cancelAnimationFrame(frame);
      resize.disconnect();
      mutations.disconnect();
      for (const event of ["scroll", "history-shift", "mapping-shift"])
        pane.removeEventListener(event, schedule);
    };
  }, [present, area]);
  const face = (
    <span className="activity-card-face">
      <span className="activity-card-content">{children}</span>
    </span>
  );
  return (
    <Button
      type="button"
      data-seq={seq.toString()}
      data-state={state}
      className={`event-detail detail-anchor ${className}`}
      ref={details.anchor}
      data-detail-present={details.present}
      data-detail-open={details.expanded}
      aria-expanded={details.expanded}
      aria-haspopup="dialog"
      aria-controls={details.controls}
      aria-describedby={timestamp}
      data-lifted={present && !!area}
      data-lifted-entered={entered}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      onPointerDown={() => {
        setPressed(true);
        details.anchor.current?.focus({ preventScroll: true });
      }}
      {...details.handlers({ title, content })}
    >
      <EventTimePopover timeMs={timeMs} id={timestamp} />
      {face}
      {present &&
        area &&
        createPortal(
          <div className="activity-card-layer">
            <div
              ref={surface}
              className="activity-card-preview"
              data-tool={className.includes("tool-activity")}
              data-entered={entered}
              data-unfolded={unfolded}
              data-hovered={hovered}
              data-detail-open={details.expanded}
              data-focused={
                focused && details.anchor.current?.matches(":focus-visible")
              }
              data-pressed={pressed}
              aria-hidden="true"
            >
              <EventTimePopover timeMs={timeMs} id={`${timestamp}-preview`} />
              {face}
              <span className="activity-card-hit-area" />
            </div>
          </div>,
          area,
        )}
    </Button>
  );
}
