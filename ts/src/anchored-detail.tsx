import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { FloatingCard } from "./card-shell";

// The overlay lives outside the virtualized rows. Only the trigger's backdrop
// expands, so opening details never changes measured message heights or range.
export function AnchoredDetail({
  anchor,
  id,
  title,
  label,
  closing,
  close,
  children,
}: {
  anchor: HTMLElement;
  id: string;
  title: string;
  label?: string;
  closing: boolean;
  close: (restore?: boolean) => void;
  children: ReactNode;
}) {
  const area = anchor.closest<HTMLElement>(".transcript-area");
  const root = useRef<HTMLElement>(null);
  const layer = useRef<HTMLDivElement>(null);
  const latestClose = useRef(close);
  latestClose.current = close;
  const [entered, setEntered] = useState(false);
  useLayoutEffect(() => {
    if (!area) return;
    const conversation = area.closest<HTMLElement>(".conversation")!;
    const pane = area.querySelector<HTMLElement>(".transcript")!;
    const virtual = anchor.closest(".virtual-messages")!;
    const row = anchor.closest(".transcript-row")!;
    let frame = 0;
    let enterFrame = 0;
    const measure = () => {
      frame = 0;
      const origin = anchor.getBoundingClientRect();
      const view = area.getBoundingClientRect();
      if (
        !anchor.isConnected ||
        origin.bottom <= view.top ||
        origin.top >= view.bottom
      ) {
        latestClose.current(false);
        return;
      }
      const card = root.current!;
      const style = getComputedStyle(card);
      // Read resolved lengths: custom properties can still contain calc().
      const overlap = parseFloat(style.paddingTop);
      const inset = parseFloat(getComputedStyle(layer.current!).paddingBottom);
      const bottom = conversation.getBoundingClientRect().bottom;
      const top = origin.bottom - view.top - overlap;
      layer.current!.style.height = `${bottom - view.top}px`;
      card.style.top = `${top}px`;
      card.style.left = `${origin.left - view.left}px`;
      card.style.width = `${origin.width}px`;
      card.style.setProperty(
        "--detail-space",
        `${Math.max(0, bottom - view.top - top - inset)}px`,
      );
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    measure();
    enterFrame = requestAnimationFrame(() => setEntered(true));
    const resize = new ResizeObserver(schedule);
    resize.observe(anchor);
    resize.observe(area);
    resize.observe(conversation);
    const mutations = new MutationObserver(schedule);
    mutations.observe(virtual, { childList: true });
    mutations.observe(row, { attributes: true, attributeFilter: ["style"] });
    for (const event of ["scroll", "history-shift", "mapping-shift"])
      pane.addEventListener(event, schedule);
    return () => {
      cancelAnimationFrame(frame);
      cancelAnimationFrame(enterFrame);
      resize.disconnect();
      mutations.disconnect();
      for (const event of ["scroll", "history-shift", "mapping-shift"])
        pane.removeEventListener(event, schedule);
    };
  }, [anchor, area]);
  if (!area) return null;
  return createPortal(
    <div ref={layer} className="anchored-detail-layer">
      <FloatingCard
        ref={root}
        id={id}
        data-card={id}
        title={title}
        className="anchored-detail"
        bodyTabIndex={0}
        role="dialog"
        aria-label={label ?? title}
        aria-hidden={closing}
        inert={closing}
        data-active={!closing}
        data-entered={entered}
        data-closing={closing}
      >
        {children}
      </FloatingCard>
    </div>,
    area,
  );
}
