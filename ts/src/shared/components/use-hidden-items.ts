import { useEffect, useState, type RefObject } from "react";

// Only unread targets are measured, at most once per frame. No polling or streams.
export function useHiddenItems(
  viewport: RefObject<HTMLDivElement | null>,
  selector?: string,
) {
  const [hidden, setHidden] = useState({ above: false, below: false });
  useEffect(() => {
    const pane = viewport.current;
    if (!pane || !selector) {
      setHidden((old) =>
        old.above || old.below ? { above: false, below: false } : old,
      );
      return;
    }
    let targets = Array.from(pane.querySelectorAll<HTMLElement>(selector));
    let frame = 0;
    const measure = () => {
      frame = 0;
      const bounds = pane.getBoundingClientRect();
      let above = false,
        below = false;
      for (const target of targets) {
        const rect = target.getBoundingClientRect();
        if (!rect.height || !rect.width) continue;
        above ||= rect.top < bounds.top - 1;
        below ||= rect.bottom > bounds.bottom + 1;
        if (above && below) break;
      }
      setHidden((old) =>
        old.above === above && old.below === below ? old : { above, below },
      );
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    const mutations = new MutationObserver(() => {
      targets = Array.from(pane.querySelectorAll<HTMLElement>(selector));
      schedule();
    });
    mutations.observe(pane, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["data-unread"],
    });
    const resize = new ResizeObserver(schedule);
    resize.observe(pane);
    if (pane.firstElementChild) resize.observe(pane.firstElementChild);
    pane.addEventListener("scroll", schedule, { passive: true });
    schedule();
    return () => {
      cancelAnimationFrame(frame);
      mutations.disconnect();
      resize.disconnect();
      pane.removeEventListener("scroll", schedule);
    };
  }, [viewport, selector]);
  return hidden;
}
