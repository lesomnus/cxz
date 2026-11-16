import { useLayoutEffect, useRef, useState } from "react";

export function useLogViewport(count: number, scope: string) {
  const viewport = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, height: 0, row: 24, header: 28 });
  const frame = useRef(0);
  function measure() {
    const node = viewport.current;
    if (!node) return;
    const row =
      node.querySelector<HTMLElement>("[data-log-row]")?.getBoundingClientRect()
        .height ?? view.row;
    const header =
      node.querySelector("thead")?.getBoundingClientRect().height ??
      view.header;
    const next = {
      top: node.scrollTop,
      height: node.clientHeight,
      row,
      header,
    };
    setView((old) =>
      Object.keys(next).every(
        (key) =>
          old[key as keyof typeof old] === next[key as keyof typeof next],
      )
        ? old
        : next,
    );
  }
  useLayoutEffect(() => {
    if (viewport.current) viewport.current.scrollTop = 0;
  }, [scope]);
  useLayoutEffect(() => {
    measure();
    const observer = new ResizeObserver(measure);
    if (viewport.current) observer.observe(viewport.current);
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame.current);
    };
  }, [count, scope]);
  const start = Math.min(
    Math.max(0, count - 1),
    Math.max(0, Math.floor((view.top - view.header) / view.row) - 20),
  );
  const end = Math.min(count, start + Math.ceil(view.height / view.row) + 40);
  return {
    viewport,
    start,
    end,
    before: start * view.row,
    after: Math.max(0, count - end) * view.row,
    scrolled() {
      cancelAnimationFrame(frame.current);
      frame.current = requestAnimationFrame(measure);
    },
  };
}
