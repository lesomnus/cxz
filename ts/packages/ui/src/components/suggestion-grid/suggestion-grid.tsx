import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { Button } from "../button/button";
import styles from "./suggestion-grid.module.css";
import { radialGridOrder, type SuggestionAnchor } from "./suggestion-layout";
export {
  radialGridOrder,
  moveRankedGridSelection,
  type SuggestionAnchor,
} from "./suggestion-layout";
export type SuggestionGridLayout = { columns: number; order: number[] };

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
  onLayoutChange,
  anchor,
  message,
  className,
}: {
  id: string;
  label: string;
  items: readonly SuggestionGridItem[];
  selected: number;
  choose: (index: number) => void;
  onColumnsChange?: (columns: number) => void;
  onLayoutChange?: (layout: SuggestionGridLayout) => void;
  anchor?: SuggestionAnchor;
  message?: string;
  className?: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  const [geometry, setGeometry] = useState({
    centers: [0],
    rowHeight: 0,
    rowGap: 0,
    padding: 0,
  });
  const columns = geometry.centers.length;
  const order = useMemo(
    () =>
      anchor
        ? radialGridOrder(
            items.length,
            geometry.centers,
            geometry.rowHeight,
            geometry.rowGap,
            geometry.padding,
            anchor,
          )
        : Array.from({ length: items.length }, (_, index) => index),
    [anchor, geometry, items.length],
  );
  useLayoutEffect(() => {
    const el = root.current!;
    const measure = () => {
      const css = getComputedStyle(el),
        bounds = el.getBoundingClientRect();
      const scale = bounds.width / el.offsetWidth || 1;
      const widths =
        css.gridTemplateColumns === "none"
          ? [bounds.width]
          : css.gridTemplateColumns
              .split(" ")
              .map((value) => parseFloat(value) * scale);
      const gap = (parseFloat(css.columnGap) || 0) * scale;
      let left = parseFloat(css.paddingLeft) * scale;
      const centers = widths.map((width) => {
        const center = left + width / 2;
        left += width + gap;
        return center;
      });
      const next = {
        centers,
        rowHeight:
          el.querySelector('[role="gridcell"]')?.getBoundingClientRect()
            .height ?? 0,
        rowGap: (parseFloat(css.rowGap) || 0) * scale,
        padding:
          parseFloat(
            anchor?.edge === "bottom" ? css.paddingBottom : css.paddingTop,
          ) * scale,
      };
      setGeometry((old) =>
        JSON.stringify(old) === JSON.stringify(next) ? old : next,
      );
      onColumnsChange?.(centers.length);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    const first = el.querySelector('[role="gridcell"]');
    if (first) observer.observe(first);
    document.fonts.addEventListener("loadingdone", measure);
    return () => {
      observer.disconnect();
      document.fonts.removeEventListener("loadingdone", measure);
    };
  }, [onColumnsChange, anchor?.edge, items.length]);
  useLayoutEffect(() => {
    onLayoutChange?.({ columns, order });
  }, [columns, order, onLayoutChange]);
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
  }, [selected, items, columns, order]);
  return (
    <div
      className={[styles.grid, className].filter(Boolean).join(" ")}
      ref={root}
      id={id}
      role="grid"
      aria-label={label}
      aria-rowcount={Math.max(1, Math.ceil(order.length / columns))}
      aria-colcount={columns}
    >
      {items.length ? (
        Array.from({ length: Math.ceil(order.length / columns) }, (_, row) => (
          <div
            key={row}
            role="row"
            aria-rowindex={row + 1}
            className={styles.row}
          >
            {order
              .slice(row * columns, (row + 1) * columns)
              .map((index, column) => {
                if (index < 0)
                  return (
                    <div
                      key={`empty-${column}`}
                      className={styles.placeholder}
                      aria-hidden="true"
                    />
                  );
                const item = items[index];
                return (
                  <Button
                    key={index}
                    id={`${id}-${index}`}
                    className={styles.item}
                    type="button"
                    role="gridcell"
                    tabIndex={-1}
                    aria-selected={index === selected}
                    aria-colindex={column + 1}
                    data-rank={index}
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
