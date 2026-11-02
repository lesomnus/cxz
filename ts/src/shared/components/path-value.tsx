import { useLayoutEffect, useRef, useState } from "react";
import { Button, Popover } from "@lesomnus/nox";
import { EllipsisMark } from "./ellipsis-mark";
import { elidePath, type ElidedPath } from "./path-elision";
import styles from "./path-value.module.css";

// Elide the middle before shortening a leaf, preserving the leading context.
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
  const [display, setDisplay] = useState<ElidedPath>({
    prefix: "",
    leaf: value,
    middle: false,
    end: false,
    separator: "",
  });
  useLayoutEffect(() => {
    const container = measure.current;
    const text = container?.querySelector("span")?.firstChild;
    const mark = container?.querySelector("svg");
    if (!anchor || !container || !text || !mark) return;
    const update = () => {
      // Measure real glyphs, including loaded fonts, rather than character counts.
      const range = document.createRange();
      const widthOf = (part: string) => {
        if (!part) return 0;
        const start = value.indexOf(part);
        range.setStart(text, start);
        range.setEnd(text, start + part.length);
        return range.getBoundingClientRect().width;
      };
      const next = elidePath(
        value,
        anchor.clientWidth,
        widthOf,
        mark.getBoundingClientRect().width,
      );
      setDisplay((current) =>
        current.prefix === next.prefix &&
        current.leaf === next.leaf &&
        current.middle === next.middle &&
        current.end === next.end &&
        current.separator === next.separator
          ? current
          : next,
      );
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(anchor);
    observer.observe(container);
    return () => observer.disconnect();
  }, [anchor, value]);
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
        <span className={styles.text}>
          {display.prefix && (
            <span className={styles.prefix}>{display.prefix}</span>
          )}
          {display.middle && (
            <>
              <EllipsisMark />
              <span>{display.separator}</span>
            </>
          )}
          <span className={styles.leaf}>{display.leaf}</span>
          {display.end && <EllipsisMark />}
        </span>
        <span ref={measure} className={styles.measure} aria-hidden="true">
          <span>{value}</span>
          <EllipsisMark />
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
