import {
  cloneElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactElement,
} from "react";
import { createPortal } from "react-dom";
import styles from "./sidebar-hint.module.css";

export function SidebarHint({
  label,
  children,
}: {
  label: string;
  children: ReactElement<{ "aria-describedby"?: string }>;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState<{ left: number; top: number }>();
  function show() {
    const anchor = root.current?.querySelector("a");
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    setPosition({
      left: rect.right + 8,
      top: Math.max(18, Math.min(innerHeight - 18, rect.top + rect.height / 2)),
    });
  }
  useEffect(() => {
    if (!position) return;
    const close = () => setPosition(undefined);
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    document.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
      document.removeEventListener("keydown", escape);
    };
  }, [position]);
  return (
    <div
      ref={root}
      className={styles.anchor}
      onPointerEnter={(event) => {
        if (event.pointerType !== "touch") show();
      }}
      onPointerLeave={() => setPosition(undefined)}
      onFocus={show}
      onBlur={() => setPosition(undefined)}
      onClick={() => setPosition(undefined)}
    >
      {cloneElement(children, {
        "aria-describedby": position ? id : undefined,
      })}
      {position &&
        createPortal(
          <span id={id} role="tooltip" className={styles.hint} style={position}>
            {label}
          </span>,
          document.body,
        )}
    </div>
  );
}
