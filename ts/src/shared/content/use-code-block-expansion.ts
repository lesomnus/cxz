import { useLayoutEffect, useMemo, useState } from "react";

// Measure only while expanded. The normal wrapper remains the toolbar anchor;
// its pre/diagram surface alone reaches the conversation viewport edges.
export function useCodeBlockExpansion(container: HTMLElement) {
  const boundary = useMemo(
    () => container.closest<HTMLElement>("[data-code-expand-boundary]"),
    [container],
  );
  const [expanded, setExpanded] = useState(false);
  useLayoutEffect(() => {
    if (!expanded || !boundary) return;
    let frame = 0;
    const measure = () => {
      frame = 0;
      const block = container.getBoundingClientRect();
      const viewport = boundary.getBoundingClientRect();
      container.style.setProperty(
        "--code-expanded-width",
        `${boundary.clientWidth}px`,
      );
      container.style.setProperty(
        "--code-expanded-left",
        `${viewport.left + boundary.clientLeft - block.left}px`,
      );
      container.dataset.codeExpanded = "true";
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    measure();
    const resize = new ResizeObserver(schedule);
    resize.observe(boundary);
    resize.observe(container);
    // Live-arrival transforms can temporarily shift the wrapper's screen box.
    const row = container.closest(".transcript-row");
    const arrived = (event: Event) => {
      if (event.target instanceof Element && event.target.parentElement === row)
        schedule();
    };
    row?.addEventListener("animationend", arrived);
    return () => {
      resize.disconnect();
      row?.removeEventListener("animationend", arrived);
      cancelAnimationFrame(frame);
      delete container.dataset.codeExpanded;
      container.style.removeProperty("--code-expanded-width");
      container.style.removeProperty("--code-expanded-left");
    };
  }, [container, boundary, expanded]);
  return {
    available: !!boundary,
    expanded,
    toggle: () => setExpanded((value) => !value),
  };
}
