import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import {
  DialogSurface,
  dialogClassName,
  DialogTrigger,
  captureDialogTrigger,
  focusedDialogTrigger,
} from "@lesomnus/cxz-ui";
import styles from "./anchored-dialog.module.css";

// A native modal focus boundary positioned beside an existing panel.
export function AnchoredDialog({
  title,
  anchor,
  close,
  busy = false,
  children,
  trigger,
}: {
  title: string;
  anchor?: HTMLElement | null;
  close: () => void;
  busy?: boolean;
  children: ReactNode;
  trigger?: HTMLElement | null;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [adjacent, setAdjacent] = useState(false);
  const [origin] = useState(() =>
    anchor
      ? trigger
        ? captureDialogTrigger(trigger)
        : focusedDialogTrigger()
      : undefined,
  );
  const id = useId();
  useLayoutEffect(() => {
    const node = dialog.current!;
    node.showModal();
    const position = () => {
      const inset =
        parseFloat(getComputedStyle(node).getPropertyValue("--space")) || 4;
      const rect = anchor?.isConnected
        ? anchor.getBoundingClientRect()
        : undefined;
      const left = Math.max(
        inset,
        Math.min(
          rect?.right ?? (window.innerWidth - node.offsetWidth) / 2,
          window.innerWidth - node.offsetWidth - inset,
        ),
      );
      node.style.left = `${left}px`;
      node.style.top = `${Math.max(inset, Math.min(rect?.top ?? inset, window.innerHeight - node.offsetHeight - inset))}px`;
      setAdjacent(!!rect && left >= rect.right);
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
      <DialogTrigger origin={origin} adjacent={adjacent} />
      <DialogSurface anchored={adjacent}>{children}</DialogSurface>
    </dialog>,
    document.body,
  );
}
