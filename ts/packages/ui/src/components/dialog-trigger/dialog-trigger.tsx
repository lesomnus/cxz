import { useLayoutEffect, useRef, type RefObject } from "react";
import styles from "./dialog-trigger.module.css";

export type DialogTriggerOrigin = {
  element: HTMLElement;
  content: HTMLElement;
  bounds: DOMRect;
};

// Capture before a menu item unmounts. Copy computed styles so ancestor-dependent
// typography and layout survive the move into the native dialog's top layer.
export function captureDialogTrigger(
  element?: HTMLElement | null,
): DialogTriggerOrigin | undefined {
  if (!element) return;
  const bounds = element.getBoundingClientRect();
  if (!bounds.width || !bounds.height) return;
  const content = element.cloneNode(true) as HTMLElement;
  const originals = [
    element,
    ...element.querySelectorAll<HTMLElement | SVGElement>("*"),
  ];
  const copies = [
    content,
    ...content.querySelectorAll<HTMLElement | SVGElement>("*"),
  ];
  originals.forEach((original, index) => {
    const copy = copies[index];
    const computed = getComputedStyle(original);
    copy.style.cssText = Array.from(computed)
      .map((key) => `${key}:${computed.getPropertyValue(key)};`)
      .join("");
    copy.style.animation = "none";
    copy.style.transition = "none";
    if (copy.classList.contains("button-content"))
      copy.style.transform = "none";
    copy.removeAttribute("id");
    copy.removeAttribute("autofocus");
    copy.removeAttribute("title");
    copy.removeAttribute("href");
    copy.setAttribute("tabindex", "-1");
    if (copy instanceof HTMLButtonElement) copy.disabled = true;
  });
  return { element, content, bounds };
}

export function focusedDialogTrigger() {
  return captureDialogTrigger(
    document.activeElement instanceof HTMLElement
      ? document.activeElement.closest<HTMLElement>("button, a[href]")
      : undefined,
  );
}

// Only an explicitly adjacent dialog retains its trigger above the backdrop.
// The source remains inert; no event handlers or focusable content are copied.
export function DialogTrigger({
  origin,
  relativeTo,
  adjacent = false,
}: {
  origin?: DialogTriggerOrigin;
  relativeTo?: RefObject<HTMLElement | null>;
  adjacent?: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = host.current;
    if (!node || !origin || !adjacent) return;
    const content = origin.content.cloneNode(true) as HTMLElement;
    Object.assign(content.style, {
      position: "relative",
      inset: "auto",
      margin: "0",
      width: "100%",
      height: "100%",
      minWidth: "0",
      minHeight: "0",
      maxWidth: "none",
      maxHeight: "none",
      transform: "none",
      pointerEvents: "none",
    });
    content.setAttribute("aria-disabled", "true");
    node.replaceChildren(content);
    let frame = 0;
    let containerObserved = false;
    const position = () => {
      const container = relativeTo?.current;
      if (relativeTo && !container) {
        node.style.visibility = "hidden";
        frame = requestAnimationFrame(position);
        return;
      }
      node.style.visibility = "visible";
      const rect = origin.element.isConnected
        ? origin.element.getBoundingClientRect()
        : origin.bounds;
      const bounds = container?.getBoundingClientRect();
      Object.assign(node.style, {
        position: container ? "absolute" : "fixed",
        left: `${rect.left - (bounds?.left ?? 0)}px`,
        top: `${rect.top - (bounds?.top ?? 0)}px`,
        width: `${rect.width}px`,
        height: `${rect.height}px`,
      });
      if (container && !containerObserved) {
        observer.observe(container);
        containerObserved = true;
      }
    };
    const observer = new ResizeObserver(position);
    position();
    if (origin.element.isConnected) observer.observe(origin.element);
    window.addEventListener("resize", position);
    window.addEventListener("scroll", position, true);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener("resize", position);
      window.removeEventListener("scroll", position, true);
      node.replaceChildren();
    };
  }, [origin, relativeTo, adjacent]);
  if (!origin || !adjacent) return null;
  return (
    <div
      ref={host}
      className={styles.trigger}
      data-dialog-trigger="true"
      aria-hidden="true"
      onPointerDown={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
    />
  );
}
