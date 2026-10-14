import { useLayoutEffect, useState, type RefObject } from "react";

import { loadPaneWidth, savePaneWidth } from "./layout-storage";

/** Resolve sizes from CSS tokens, keeping the user's width across viewport changes. */
export function usePaneWidth(
  area: RefObject<HTMLElement | null>,
  prefix: "session" | "file",
  enabled = true,
) {
  const [chosen, choose] = useState<number | undefined>(() =>
    loadPaneWidth(prefix),
  );
  const [bounds, setBounds] = useState({ initial: 0, min: 0, max: 0 });
  useLayoutEffect(() => {
    const element = area.current;
    if (!enabled || !element) return;
    const measure = () => {
      const css = getComputedStyle(element);
      const token = (name: string) =>
        parseFloat(css.getPropertyValue(name)) || 0;
      const available = Math.max(
        0,
        element.clientWidth - token("--ui-resize-handle-size"),
      );
      const otherMin = token(`--${prefix}-other-min-width`);
      const min = Math.min(
        token(`--${prefix}-pane-min-width`),
        available / 2,
        Math.max(0, available - otherMin),
      );
      const max = Math.max(min, available - otherMin);
      const initial = Math.max(
        min,
        Math.min(max, token(`--${prefix}-pane-width`)),
      );
      setBounds((current) =>
        current.initial === initial &&
        current.min === min &&
        current.max === max
          ? current
          : { initial, min, max },
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [area, prefix, enabled]);
  return {
    value: Math.max(bounds.min, Math.min(bounds.max, chosen ?? bounds.initial)),
    min: bounds.min,
    max: bounds.max,
    onChange: (width: number) => {
      choose(width);
      savePaneWidth(prefix, width);
    },
    onReset: () => {
      choose(undefined);
      savePaneWidth(prefix, undefined);
    },
  };
}
