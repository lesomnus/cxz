import { useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { DialogSurface, dialogClassName } from "@lesomnus/cxz-ui";
import styles from "./anchored-dialog.module.css";

// A native modal focus boundary positioned beside an existing panel.
export function AnchoredDialog({
  title,
  anchor,
  close,
  busy = false,
  children,
}: {
  title: string;
  anchor?: HTMLElement | null;
  close: () => void;
  busy?: boolean;
  children: ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  useLayoutEffect(() => {
    const node = dialog.current!;
    node.showModal();
    const position = () => {
      const inset =
        parseFloat(getComputedStyle(node).getPropertyValue("--space")) || 4;
      const rect = anchor?.getBoundingClientRect();
      node.style.left = `${Math.max(inset, Math.min(rect?.right ?? (window.innerWidth - node.offsetWidth) / 2, window.innerWidth - node.offsetWidth - inset))}px`;
      node.style.top = `${Math.max(inset, Math.min(rect?.top ?? inset, window.innerHeight - node.offsetHeight - inset))}px`;
    };
    position();
    window.addEventListener("resize", position);
    const observer = new ResizeObserver(position);
    observer.observe(node);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
      node.close();
    };
  }, [anchor]);
  return createPortal(
    <dialog
      ref={dialog}
      className={`${dialogClassName} ${styles.dialog}`}
      aria-labelledby={id}
      aria-busy={busy}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) close();
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget || busy) return;
        const rect = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX < rect.left ||
          event.clientX > rect.right ||
          event.clientY < rect.top ||
          event.clientY > rect.bottom
        )
          close();
      }}
    >
      <span id={id} className={styles.label}>
        {title}
      </span>
      <DialogSurface anchored>{children}</DialogSurface>
    </dialog>,
    document.body,
  );
}
