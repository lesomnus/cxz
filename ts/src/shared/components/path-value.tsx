import { useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { Button, Popover } from "@lesomnus/nox";
import styles from "./path-value.module.css";

// Shrink the ancestors before truncating the final file/directory name.
export function PathValue({
  value,
  label,
  copying,
  onCopy,
}: {
  value: string;
  label: string;
  copying: boolean;
  onCopy: () => Promise<void>;
}) {
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  const measure = useRef<HTMLSpanElement>(null);
  const [collapsed, setCollapsed] = useState(false);
  useLayoutEffect(() => {
    const text = measure.current;
    if (!anchor || !text) return;
    const update = () =>
      setCollapsed(text.getBoundingClientRect().width > anchor.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(anchor);
    observer.observe(text);
    return () => observer.disconnect();
  }, [anchor, value]);
  const parts = value.match(/^(.*[/\\])([^/\\]+[/\\]*)$/);
  const parent = parts?.[1] ?? "";
  const leaf = parts?.[2] ?? value;
  const shownParent =
    collapsed && parent.length > 4 ? `...${parent.at(-1)}` : parent;
  const parentMin = collapsed ? shownParent.length : Math.min(parent.length, 2);
  return (
    <>
      <Button
        ref={setAnchor}
        type="button"
        effect="none"
        className={styles.path}
        aria-label={label}
        aria-busy={copying || undefined}
        onClick={onCopy}
        onPointerEnter={(event) => {
          if (event.pointerType !== "touch") setOpen(true);
        }}
        onPointerLeave={() => {
          if (document.activeElement !== anchor) setOpen(false);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
      >
        <span
          className={styles.text}
          style={{ "--path-parent-min": `${parentMin}ch` } as CSSProperties}
        >
          {shownParent && <span className={styles.parent}>{shownParent}</span>}
          <span className={styles.leaf}>{leaf}</span>
        </span>
        <span ref={measure} className={styles.measure} aria-hidden="true">
          {value}
        </span>
      </Button>
      <Popover
        anchor={anchor}
        open={open}
        onOpenChange={setOpen}
        placement="in-place"
        content={value}
      />
    </>
  );
}
