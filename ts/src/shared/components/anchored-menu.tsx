import { useLayoutEffect, useRef, type ReactNode } from "react";
import {
  DialogReveal,
  MenuSurface,
  MenuFrame,
  snapMenuPosition,
} from "@lesomnus/nox";
import styles from "./anchored-menu.module.css";

/** Headerless anchored menus share placement, motion and dismissal behavior. */
export function AnchoredMenu({
  id,
  anchor,
  label,
  children,
  header,
  footer,
  placement = "vertical",
  gap,
  autoFocus = true,
  className = "",
  role = "menu",
  close,
}: {
  id: string;
  anchor: HTMLElement;
  label: string;
  children: ReactNode;
  header?: ReactNode;
  footer?: ReactNode;
  /** `overlay` covers the anchor rather than sitting beside it. */
  placement?: "vertical" | "right" | "overlay";
  gap?: number | ((anchor: HTMLElement) => number);
  autoFocus?: boolean;
  className?: string;
  role?: "menu" | "dialog";
  close: (restoreFocus?: boolean) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = root.current;
    if (!node) return;
    let frame = 0;
    const position = () => {
      if (!anchor.isConnected) return close();
      const rect = anchor.getBoundingClientRect();
      const space =
        parseFloat(getComputedStyle(node).getPropertyValue("--ui-space")) || 4;
      const distance = typeof gap === "function" ? gap(anchor) : (gap ?? space);
      const available = Math.max(0, rect.top - distance - space);
      const above = available >= Math.min(120, innerHeight / 3);
      const budget = Math.max(
        0,
        Math.min(
          innerHeight / 2,
          placement === "right"
            ? innerHeight - 2 * space
            : placement === "overlay"
              ? innerHeight - 2 * space
              : above
                ? available
                : innerHeight - rect.bottom - distance - space,
        ),
      );
      const frame = getComputedStyle(node);
      const chrome =
        parseFloat(frame.paddingTop) +
        parseFloat(frame.paddingBottom) +
        parseFloat(frame.borderTopWidth) +
        parseFloat(frame.borderBottomWidth) +
        [...node.querySelectorAll<HTMLElement>("[data-menu-fixed]")].reduce(
          (sum, region) => sum + region.offsetHeight,
          0,
        );
      node.style.maxHeight = `${budget}px`;
      node.style.maxWidth =
        placement === "right"
          ? `${Math.max(0, innerWidth - rect.right - distance - space)}px`
          : "";
      node.style.setProperty(
        "--menu-scroll-max-height",
        `${Math.max(0, budget - chrome)}px`,
      );
      const box = node.getBoundingClientRect();
      node.style.left = `${placement === "right" ? rect.right + distance : Math.max(space, Math.min(rect.left, innerWidth - box.width - space))}px`;
      node.style.top = `${
        placement === "right"
          ? Math.max(
              space,
              Math.min(rect.top, innerHeight - box.height - space),
            )
          : // A menu that covers its trigger starts where the trigger does, so
            // the value it replaces is the value it is about.
            placement === "overlay"
            ? Math.max(
                space,
                Math.min(rect.top, innerHeight - box.height - space),
              )
            : above
              ? rect.top - box.height - distance
              : rect.bottom + distance
      }px`;
      snapMenuPosition(node);
      node.style.visibility = "visible";
    };
    const schedulePosition = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        position();
      });
    };
    position();
    if (autoFocus)
      node
        // Checkable items are items: a menu of radios would otherwise open
        // with nothing focused and ignore the arrow keys below.
        .querySelector<HTMLElement>('[role^="menuitem"]')
        ?.focus({ preventScroll: true });
    const outside = (event: PointerEvent) => {
      if (
        !node.contains(event.target as Node) &&
        !anchor.contains(event.target as Node)
      )
        close();
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      close(true);
    };
    const scroll = (event: Event) => {
      if (!node.contains(event.target as Node)) schedulePosition();
    };
    const visibility = new IntersectionObserver(([entry]) => {
      if (!entry.isIntersecting) close();
    });
    visibility.observe(anchor);
    const observer = new ResizeObserver(schedulePosition);
    observer.observe(node);
    observer.observe(anchor);
    window.addEventListener("resize", schedulePosition);
    document.addEventListener("pointerdown", outside, true);
    document.addEventListener("keydown", escape, true);
    document.addEventListener("scroll", scroll, true);
    return () => {
      observer.disconnect();
      visibility.disconnect();
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", schedulePosition);
      document.removeEventListener("pointerdown", outside, true);
      document.removeEventListener("keydown", escape, true);
      document.removeEventListener("scroll", scroll, true);
    };
  }, [anchor, close, placement, autoFocus, gap]);
  return (
    <MenuSurface
      id={id}
      anchor={anchor}
      ref={root}
      className={`${styles.menu} ${className}`}
      role={role}
      aria-label={label}
      onKeyDown={(event) => {
        if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key))
          return;
        event.preventDefault();
        const items = [
          ...event.currentTarget.querySelectorAll<HTMLElement>(
            '[role^="menuitem"]:not(:disabled):not([aria-disabled="true"])',
          ),
        ];
        const current = items.indexOf(document.activeElement as HTMLElement);
        const next =
          event.key === "Home"
            ? 0
            : event.key === "End"
              ? items.length - 1
              : (current + (event.key === "ArrowUp" ? -1 : 1) + items.length) %
                items.length;
        items[next]?.focus({ preventScroll: true });
        items[next]?.scrollIntoView({ block: "nearest" });
      }}
      onBlur={(event) => {
        const target = event.relatedTarget as Node | null;
        if (!event.currentTarget.contains(target) && !anchor.contains(target))
          close();
      }}
    >
      <DialogReveal
        from={
          placement === "right"
            ? "left"
            : placement === "overlay"
              ? "top-left"
              : "bottom-left"
        }
        className={styles.content}
        contentClassName={styles.content}
      >
        <MenuFrame header={header} footer={footer}>
          {children}
        </MenuFrame>
      </DialogReveal>
    </MenuSurface>
  );
}
