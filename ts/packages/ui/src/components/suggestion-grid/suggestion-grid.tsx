import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { Button } from "../button/button";
import styles from "./suggestion-grid.module.css";

export type SuggestionGridItem = {
  label: string;
  content?: ReactNode;
  title?: string;
  color?: string;
};

// Move by the visible row/column, without wrapping across a row boundary.
export function moveGridSelection(
  index: number,
  count: number,
  columns: number,
  key: string,
) {
  const width = Math.max(1, columns);
  const row = Math.floor(index / width),
    column = index % width;
  if (key === "ArrowLeft") return column ? index - 1 : index;
  if (key === "ArrowRight")
    return column < width - 1 && index + 1 < count ? index + 1 : index;
  if (key === "ArrowUp") return row ? index - width : index;
  if (key === "ArrowDown")
    return (row + 1) * width < count
      ? Math.min(index + width, count - 1)
      : index;
  return index;
}

/** A responsive candidate matrix; the host retains focus and handles acceptance. */
export function SuggestionGrid({
  id,
  label,
  items,
  selected,
  choose,
  onColumnsChange,
  message,
}: {
  id: string;
  label: string;
  items: readonly SuggestionGridItem[];
  selected: number;
  choose: (index: number) => void;
  onColumnsChange: (columns: number) => void;
  message?: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(1);
  useLayoutEffect(() => {
    const el = root.current!;
    const measure = () => {
      const tracks = getComputedStyle(el).gridTemplateColumns;
      const next = tracks === "none" ? 1 : tracks.split(" ").length;
      setColumns(next);
      onColumnsChange(next);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [onColumnsChange]);
  useLayoutEffect(() => {
    const el = root.current!;
    const item = el.querySelector<HTMLElement>('[aria-selected="true"]');
    if (!item) return;
    const ensureVisible = () => {
      const viewport = el.getBoundingClientRect(),
        bounds = item.getBoundingClientRect();
      if (bounds.top < viewport.top) el.scrollTop -= viewport.top - bounds.top;
      else if (bounds.bottom > viewport.bottom)
        el.scrollTop += bounds.bottom - viewport.bottom;
    };
    ensureVisible();
    const observer = new ResizeObserver(ensureVisible);
    observer.observe(el);
    return () => observer.disconnect();
  }, [selected, items, columns]);
  return (
    <div
      className={styles.grid}
      ref={root}
      id={id}
      role="grid"
      aria-label={label}
      aria-rowcount={Math.max(1, Math.ceil(items.length / columns))}
      aria-colcount={columns}
    >
      {items.length ? (
        Array.from({ length: Math.ceil(items.length / columns) }, (_, row) => (
          <div key={row} role="row" className={styles.row}>
            {items
              .slice(row * columns, (row + 1) * columns)
              .map((item, column) => {
                const index = row * columns + column;
                return (
                  <Button
                    key={index}
                    id={`${id}-${index}`}
                    className={styles.item}
                    type="button"
                    role="gridcell"
                    tabIndex={-1}
                    aria-selected={index === selected}
                    aria-label={item.label}
                    title={item.title ?? item.label}
                    style={
                      { "--suggestion-color": item.color } as CSSProperties
                    }
                    onPointerDown={(event) => event.preventDefault()}
                    onClick={() => choose(index)}
                  >
                    {item.content ?? item.label}
                  </Button>
                );
              })}
          </div>
        ))
      ) : (
        <div role="row" className={styles.empty}>
          <div role="gridcell">
            <span role="status" title={message}>
              {message}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
