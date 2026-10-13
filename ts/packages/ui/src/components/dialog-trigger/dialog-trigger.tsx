import { useLayoutEffect, useRef } from "react";
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

// This visual copy is inside the modal top layer, above its backdrop. The source
// remains inert in the page; no event handlers or focusable content are copied.
export function DialogTrigger({ origin }: { origin?: DialogTriggerOrigin }) {
  const host = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = host.current;
    if (!node || !origin) return;
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
    const position = () => {
      const rect = origin.element.isConnected
        ? origin.element.getBoundingClientRect()
        : origin.bounds;
      Object.assign(node.style, {
        left: `${rect.left}px`,
        top: `${rect.top}px`,
        width: `${rect.width}px`,
        height: `${rect.height}px`,
      });
    };
    position();
    const observer = new ResizeObserver(position);
    if (origin.element.isConnected) observer.observe(origin.element);
    window.addEventListener("resize", position);
    window.addEventListener("scroll", position, true);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
      window.removeEventListener("scroll", position, true);
      node.replaceChildren();
    };
  }, [origin]);
  if (!origin) return null;
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
