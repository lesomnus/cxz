import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { CompactNumber, MenuSurface } from "@lesomnus/nox";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import type { FileSizeReader } from "./file-size";
import styles from "./file-link-hint.module.css";

export function FileLinkHint({
  anchor,
  path,
  readSize,
  close,
}: {
  anchor: HTMLAnchorElement;
  path: string;
  readSize?: FileSizeReader;
  close: () => void;
}) {
  const { locale } = useLocale();
  const id = useId();
  const hint = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<bigint>();
  const [failed, setFailed] = useState(!readSize);
  const [position, setPosition] = useState<{ left: number; top: number }>();
  useEffect(() => {
    if (!readSize) return;
    const controller = new AbortController();
    void readSize(path, controller.signal).then(
      (value) => {
        if (!controller.signal.aborted) setSize(value);
      },
      () => {
        if (!controller.signal.aborted) setFailed(true);
      },
    );
    return () => controller.abort();
  }, [path, readSize]);
  useLayoutEffect(() => {
    const element = hint.current;
    if (!element || !anchor.isConnected) return;
    const place = () => {
      const box = element.getBoundingClientRect();
      const rect = anchor.getBoundingClientRect();
      const gap = parseFloat(getComputedStyle(element).paddingLeft) || 0;
      setPosition({
        left: Math.max(gap, Math.min(rect.left, innerWidth - box.width - gap)),
        top: Math.max(
          gap,
          rect.top - box.height - gap >= gap
            ? rect.top - box.height - gap
            : Math.min(rect.bottom + gap, innerHeight - box.height - gap),
        ),
      });
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(element);
    anchor.setAttribute("aria-describedby", id);
    return () => {
      observer.disconnect();
      if (anchor.getAttribute("aria-describedby") === id)
        anchor.removeAttribute("aria-describedby");
    };
  }, [anchor, id]);
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    document.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
      document.removeEventListener("keydown", escape);
    };
  }, [close]);
  return (
    <MenuSurface
      anchor={anchor}
      ref={hint}
      id={id}
      role="tooltip"
      className={styles.hint}
      style={{
        ...position,
        visibility: position ? "visible" : "hidden",
        fontFamily: "var(--ui-font-mono)",
        fontSize: "12px",
        lineHeight: 1.5,
      }}
    >
      <span className={styles.size} aria-busy={size === undefined && !failed}>
        <CompactNumber
          value={size === undefined ? NaN : Number(size)}
          label={
            size !== undefined
              ? t("{size} bytes", { size: size.toLocaleString(locale) })
              : failed
                ? t("Size unavailable")
                : t("Loading…")
          }
        />
        <span className={styles.unit} aria-hidden="true">
          B
        </span>
      </span>
      <span className={styles.path}>{path}</span>
    </MenuSurface>
  );
}
