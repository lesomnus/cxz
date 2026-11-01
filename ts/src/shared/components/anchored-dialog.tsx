import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Dialog,
  DialogTrigger,
  captureDialogTrigger,
  focusedDialogTrigger,
} from "@lesomnus/nox";
import styles from "./anchored-dialog.module.css";

// A native modal focus boundary, centered unless positioned beside a panel.
export function AnchoredDialog({
  title,
  anchor,
  close,
  busy = false,
  children,
  trigger,
  centered = false,
  open,
  onClosed,
}: {
  title: string;
  centered?: boolean;
  anchor?: HTMLElement | null;
  close: () => void;
  busy?: boolean;
  children: ReactNode;
  trigger?: HTMLElement | null;
  open: boolean;
  onClosed: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [adjacent, setAdjacent] = useState(false);
  const [origin] = useState(() =>
    !centered && anchor
      ? trigger
        ? captureDialogTrigger(trigger)
        : focusedDialogTrigger()
      : undefined,
  );
  const id = useId();
  useLayoutEffect(() => {
    const node = dialog.current!;
    const position = () => {
      const inset =
        parseFloat(getComputedStyle(node).getPropertyValue("--space")) || 4;
      const rect =
        !centered && anchor?.isConnected
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
      node.style.top = `${Math.max(inset, Math.min(rect?.top ?? (window.innerHeight - node.offsetHeight) / 2, window.innerHeight - node.offsetHeight - inset))}px`;
      setAdjacent(!!rect && left >= rect.right);
    };
    position();
    window.addEventListener("resize", position);
    const observer = new ResizeObserver(position);
    observer.observe(node);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
    };
  }, [anchor, centered]);
  return (
    <Dialog
      ref={dialog}
      open={open}
      onClose={close}
      onClosed={onClosed}
      dismissible={!busy}
      frame={false}
      anchored={adjacent}
      decoration={<DialogTrigger origin={origin} adjacent={adjacent} />}
      className={styles.dialog}
      aria-labelledby={id}
      aria-busy={busy}
    >
      <span id={id} className={styles.label}>
        {title}
      </span>
      {children}
    </Dialog>
  );
}
