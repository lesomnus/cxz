import { useCallback, useLayoutEffect, useRef, type RefObject } from "react";
import { smoothStep } from "./scroll-physics";

// Retarget a running frame loop instead of restarting a CSS transition on every
// scroll event. Painting the handle never schedules a transcript React render.
export function useScrollThumb(node: RefObject<HTMLElement | null>) {
  const frame = useRef(0);
  const motion = useRef<{ top: number; target: number; time: number } | null>(
    null,
  );
  const response = useRef(28);
  const reduced = useRef<MediaQueryList | null>(null);
  const stop = useCallback(() => {
    cancelAnimationFrame(frame.current);
    frame.current = 0;
  }, []);
  const move = useCallback(
    (target: number, immediate = false) => {
      if (!node.current) return;
      if (immediate || reduced.current?.matches || !motion.current) {
        stop();
        motion.current = { top: target, target, time: performance.now() };
        node.current.style.top = `${target}px`;
        return;
      }
      motion.current.target = target;
      if (frame.current || motion.current.top === target) return;
      motion.current.time = performance.now();
      const tick = (time: number) => {
        frame.current = 0;
        const sample = motion.current;
        if (!sample || !node.current) return;
        sample.top = smoothStep(
          sample.top,
          sample.target,
          Math.min(64, time - sample.time),
          response.current,
          0.05,
        );
        sample.time = time;
        node.current.style.top = `${sample.top}px`;
        if (sample.top !== sample.target)
          frame.current = requestAnimationFrame(tick);
      };
      frame.current = requestAnimationFrame(tick);
    },
    [node, stop],
  );
  useLayoutEffect(() => {
    if (node.current) {
      const token = getComputedStyle(node.current)
        .getPropertyValue("--scroll-thumb-response-duration")
        .trim();
      const duration = parseFloat(token) * (token.endsWith("ms") ? 1 : 1000);
      if (duration > 0) response.current = duration;
    }
    const media = matchMedia("(prefers-reduced-motion: reduce)");
    reduced.current = media;
    const changed = () => {
      if (media.matches && motion.current) move(motion.current.target, true);
    };
    media.addEventListener("change", changed);
    return () => {
      media.removeEventListener("change", changed);
      stop();
      motion.current = null;
    };
  }, [node, move, stop]);
  return { move, stop };
}
