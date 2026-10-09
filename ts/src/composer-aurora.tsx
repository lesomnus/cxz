import { useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

// Paint behind the transcript and composer surfaces so their backdrop filters
// sample the glow. Measure only layout changes; the fields drift entirely in CSS.
export function ComposerAurora({ active }: { active: boolean }) {
  const origin = useRef<HTMLSpanElement>(null);
  const field = useRef<HTMLDivElement>(null);
  const [anchor, setAnchor] = useState<{
    toolbar: HTMLElement;
    conversation: HTMLElement;
  }>();
  useLayoutEffect(() => {
    const toolbar = origin.current!.parentElement!;
    const conversation = toolbar.closest<HTMLElement>(".conversation")!;
    setAnchor({ toolbar, conversation });
  }, []);
  useLayoutEffect(() => {
    if (!anchor) return;
    const { toolbar, conversation } = anchor;
    const measure = () => {
      const bar = toolbar.getBoundingClientRect();
      const view = conversation.getBoundingClientRect();
      const node = field.current;
      if (!node) return;
      node.style.left = `${bar.left - view.left}px`;
      node.style.top = `${bar.top - view.top}px`;
      node.style.width = `${bar.width}px`;
    };
    measure();
    const resize = new ResizeObserver(measure);
    for (const node of [
      toolbar,
      conversation,
      toolbar.closest(".composer")!,
      conversation.querySelector(".transcript-area")!,
    ])
      resize.observe(node);
    return () => resize.disconnect();
  }, [anchor]);
  return (
    <>
      <span ref={origin} hidden aria-hidden="true" />
      {anchor &&
        createPortal(
          <div
            ref={field}
            className="composer-aurora"
            data-active={active}
            aria-hidden="true"
          >
            <span />
            <span />
            <span />
          </div>,
          anchor.conversation,
        )}
    </>
  );
}
