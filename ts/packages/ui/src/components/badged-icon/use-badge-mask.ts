import { useLayoutEffect, useRef, type RefObject } from "react";

type Nodes = {
  root: RefObject<HTMLSpanElement | null>;
  icon: RefObject<HTMLSpanElement | null>;
  overlay: RefObject<HTMLSpanElement | null>;
  mask: RefObject<SVGMaskElement | null>;
  full: RefObject<SVGRectElement | null>;
  ellipse: RefObject<SVGEllipseElement | null>;
  rectangle: RefObject<SVGRectElement | null>;
};
type Options = {
  present: boolean;
  shape: "ellipse" | "rounded-rectangle";
  gap?: number;
  radius?: number;
};
const geometryProperties = new Set([
  "left",
  "right",
  "top",
  "bottom",
  "inset",
  "transform",
  "translate",
  "scale",
  "rotate",
  "width",
  "height",
  "margin",
  "marginLeft",
  "marginTop",
]);

function strokeGap(icon: HTMLElement, scale: number) {
  for (const element of icon.querySelectorAll<SVGGraphicsElement>(
    "svg path, svg circle, svg rect, svg line, svg polyline, svg polygon, svg ellipse",
  )) {
    const style = getComputedStyle(element);
    const width = parseFloat(style.strokeWidth);
    if (style.stroke === "none" || !width) continue;
    const matrix = element.getScreenCTM();
    if (!matrix) continue;
    return style.vectorEffect === "non-scaling-stroke"
      ? width / scale
      : (width *
          Math.max(
            Math.hypot(matrix.a, matrix.b),
            Math.hypot(matrix.c, matrix.d),
          )) /
          scale;
  }
  return (
    parseFloat(
      getComputedStyle(icon).getPropertyValue("--ui-badged-icon-gap"),
    ) || 0
  );
}

// Position is measured, so changing a badge class/style never rewrites icon
// paths. Idle instances have no animation loop. Moving CSS animations alone
// request frames, and every observer/listener is released on unmount.
export function useBadgeMask(nodes: Nodes, options: Options) {
  const latest = useRef(options);
  latest.current = options;
  const measure = useRef<() => void>(() => {});
  useLayoutEffect(() => {
    const {
      root: r,
      icon: i,
      overlay: o,
      mask: m,
      full: f,
      ellipse: e,
      rectangle: q,
    } = nodes;
    const root = r.current,
      icon = i.current,
      badge = o.current;
    const mask = m.current,
      full = f.current,
      ellipse = e.current,
      rectangle = q.current;
    if (
      !options.present ||
      !root ||
      !icon ||
      !badge ||
      !mask ||
      !full ||
      !ellipse ||
      !rectangle
    )
      return;
    let frame = 0;
    let previous = "";
    const moving = () =>
      badge
        .getAnimations({ subtree: true })
        .some(
          (animation) =>
            animation.playState === "running" &&
            animation.effect instanceof KeyframeEffect &&
            animation.effect
              .getKeyframes()
              .some((keyframe) =>
                Object.keys(keyframe).some((key) =>
                  geometryProperties.has(key),
                ),
              ),
        );
    const update = () => {
      const config = latest.current;
      const box = icon.getBoundingClientRect(),
        overlay = badge.getBoundingClientRect();
      const style = getComputedStyle(icon);
      const width = parseFloat(style.width) || icon.offsetWidth;
      const height = parseFloat(style.height) || icon.offsetHeight;
      if (!width || !height || !box.width || !box.height) return;
      const sx = box.width / width,
        sy = box.height / height;
      const x = (overlay.left - box.left) / sx,
        y = (overlay.top - box.top) / sy;
      const w = overlay.width / sx,
        h = overlay.height / sy;
      const gap = Math.max(0, config.gap ?? strokeGap(icon, Math.max(sx, sy)));
      const element =
        badge.firstElementChild instanceof HTMLElement
          ? badge.firstElementChild
          : badge;
      const corners =
        getComputedStyle(element).borderTopLeftRadius.split(/\s+/);
      const corner = (value: string, extent: number) =>
        value.endsWith("%")
          ? (parseFloat(value) * extent) / 100
          : parseFloat(value) || 0;
      const rx = Math.min(
        w / 2,
        Math.max(0, config.radius ?? corner(corners[0], w)),
      );
      const ry = Math.min(
        h / 2,
        Math.max(0, config.radius ?? corner(corners[1] ?? corners[0], h)),
      );
      const key =
        [width, height, x, y, w, h, gap, rx, ry]
          .map((value) => value.toFixed(3))
          .join(",") + config.shape;
      if (key === previous) return;
      previous = key;
      const set = (node: Element, values: Record<string, number>) => {
        for (const [name, value] of Object.entries(values))
          node.setAttribute(name, value.toFixed(3));
      };
      set(mask, { width, height });
      set(full, { width, height });
      const visible = w > 0 && h > 0;
      ellipse.style.display =
        visible && config.shape === "ellipse" ? "" : "none";
      rectangle.style.display =
        visible && config.shape === "rounded-rectangle" ? "" : "none";
      if (config.shape === "ellipse")
        set(ellipse, {
          cx: x + w / 2,
          cy: y + h / 2,
          rx: w / 2,
          ry: h / 2,
          "stroke-width": gap * 2,
        });
      else
        set(rectangle, {
          x,
          y,
          width: w,
          height: h,
          rx,
          ry,
          "stroke-width": gap * 2,
        });
    };
    const tick = () => {
      frame = 0;
      if (document.hidden) return;
      update();
      if (moving()) frame = requestAnimationFrame(tick);
    };
    const schedule = () => {
      if (!frame && !document.hidden) frame = requestAnimationFrame(tick);
    };
    const motion = (event: Event) => {
      if (event.target instanceof Node && badge.contains(event.target))
        schedule();
    };
    measure.current = update;
    update();
    const resize = new ResizeObserver(update);
    resize.observe(icon);
    resize.observe(badge);
    const mutation = new MutationObserver(schedule);
    mutation.observe(root, { attributes: true });
    mutation.observe(icon, {
      attributes: true,
      subtree: true,
      childList: true,
    });
    mutation.observe(badge, {
      attributes: true,
      subtree: true,
      childList: true,
    });
    const events = [
      "transitionrun",
      "transitionend",
      "transitioncancel",
      "animationstart",
      "animationiteration",
      "animationend",
      "animationcancel",
    ];
    for (const event of events) root.addEventListener(event, motion);
    root.addEventListener("pointerenter", schedule);
    root.addEventListener("pointerleave", schedule);
    window.addEventListener("resize", schedule);
    document.addEventListener("visibilitychange", schedule);
    return () => {
      measure.current = () => {};
      cancelAnimationFrame(frame);
      resize.disconnect();
      mutation.disconnect();
      for (const event of events) root.removeEventListener(event, motion);
      root.removeEventListener("pointerenter", schedule);
      root.removeEventListener("pointerleave", schedule);
      window.removeEventListener("resize", schedule);
      document.removeEventListener("visibilitychange", schedule);
    };
  }, [options.present]);
  useLayoutEffect(() => {
    measure.current();
  });
}
