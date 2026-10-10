import { useLayoutEffect, useRef, type ReactNode } from "react";
import { clamp } from "./scroll-physics";

// Keep native scrolling and paint only the edges where content is hidden.
// Changes to scroll position/motion do not re-render the scrollable children.
export function ScrollFade({
  children,
  viewportClassName = "",
}: {
  children: ReactNode;
  viewportClassName?: string;
}) {
  const region = useRef<HTMLDivElement>(null);
  const pane = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const root = region.current!;
    const viewport = pane.current!;
    let frame = 0;
    let lastTop = viewport.scrollTop;
    let lastTime = performance.now();
    let strength = 0;
    const paintMotion = (value: number) => {
      root.style.setProperty(
        "--scroll-fade-top-motion",
        String(Math.max(0, value)),
      );
      root.style.setProperty(
        "--scroll-fade-bottom-motion",
        String(Math.max(0, -value)),
      );
    };
    const measure = () => {
      const max = Math.max(0, viewport.scrollHeight - viewport.clientHeight);
      const top = clamp(viewport.scrollTop, 0, max);
      root.style.setProperty("--scroll-fade-top-hidden", `${top}px`);
      root.style.setProperty("--scroll-fade-bottom-hidden", `${max - top}px`);
      root.style.setProperty("--scroll-fade-top-opacity", top > 0 ? "1" : "0");
      root.style.setProperty(
        "--scroll-fade-bottom-opacity",
        max > top ? "1" : "0",
      );
    };
    const settle = (time: number) => {
      const value = strength * Math.exp(-(time - lastTime) / 110);
      paintMotion(Math.abs(value) < 0.01 ? 0 : value);
      if (Math.abs(value) >= 0.01) frame = requestAnimationFrame(settle);
    };
    const scroll = () => {
      const time = performance.now();
      const delta = viewport.scrollTop - lastTop;
      if (delta) {
        strength = clamp(delta / clamp(time - lastTime, 16, 48) / 1.5, -1, 1);
        lastTime = time;
        paintMotion(strength);
        cancelAnimationFrame(frame);
        frame = requestAnimationFrame(settle);
      }
      lastTop = viewport.scrollTop;
      measure();
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(viewport);
    observer.observe(content.current!);
    viewport.addEventListener("scroll", scroll, { passive: true });
    return () => {
      observer.disconnect();
      viewport.removeEventListener("scroll", scroll);
      cancelAnimationFrame(frame);
    };
  }, []);
  return (
    <div ref={region} className="scroll-fade-region">
      <div ref={pane} className={`scroll-fade-viewport ${viewportClassName}`}>
        <div ref={content} className="scroll-fade-content">
          {children}
        </div>
      </div>
      <div
        className="scroll-edge-fade scroll-edge-fade-top"
        aria-hidden="true"
      />
      <div
        className="scroll-edge-fade scroll-edge-fade-bottom"
        aria-hidden="true"
      />
    </div>
  );
}
