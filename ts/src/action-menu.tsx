import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "./button";

export type ActionMenuItem = {
  label: string;
  icon?: ReactNode;
  shortcut?: string;
  checked?: boolean;
  disabled?: boolean;
  danger?: boolean;
  run: () => void;
};

export function ActionMenu({
  label,
  items = [],
  groups,
  disabled = false,
  placement = "below",
  status,
}: {
  label: string;
  items?: ActionMenuItem[];
  groups?: { label: string; items: ActionMenuItem[] }[];
  disabled?: boolean;
  placement?: "above" | "below";
  status?: string;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const id = useId();
  function close(restore = false) {
    setOpen(false);
    if (restore)
      trigger.current?.querySelector("button")?.focus({ preventScroll: true });
  }
  useLayoutEffect(() => {
    if (!open || !menu.current) return;
    const popup = menu.current;
    const position = () => {
      const bounds = trigger.current!.getBoundingClientRect();
      const gap = 4,
        inset = 8;
      const above = Math.max(0, bounds.top - gap - inset);
      const below = Math.max(
        0,
        window.innerHeight - bounds.bottom - gap - inset,
      );
      const upward =
        placement === "above"
          ? above >= popup.scrollHeight || above > below
          : below < popup.scrollHeight && above > below;
      popup.style.maxHeight = `${Math.min(window.innerHeight / 2, upward ? above : below)}px`;
      popup.style.top = `${upward ? Math.max(inset, bounds.top - gap - popup.offsetHeight) : bounds.bottom + gap}px`;
      popup.style.right = `${Math.max(inset, window.innerWidth - bounds.right)}px`;
    };
    position();
    popup
      .querySelector<HTMLButtonElement>("button:not(:disabled)")
      ?.focus({ preventScroll: true });
    const resize = new ResizeObserver(position);
    resize.observe(popup);
    window.addEventListener("scroll", position, true);
    return () => {
      resize.disconnect();
      window.removeEventListener("scroll", position, true);
    };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!trigger.current?.contains(target) && !menu.current?.contains(target))
        close();
    };
    const resize = () => close();
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", resize);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", resize);
    };
  }, [open]);
  return (
    <div className="action-menu" ref={trigger}>
      <Button
        className="toolbar-button"
        type="button"
        disabled={disabled}
        aria-label={label}
        aria-description={status}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={() => setOpen(!open)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        <svg
          width="18"
          height="18"
          viewBox="0 0 18 18"
          aria-hidden="true"
          fill="currentColor"
        >
          <circle cx="4" cy="9" r="1.5" />
          <circle cx="9" cy="9" r="1.5" />
          <circle cx="14" cy="9" r="1.5" />
        </svg>
      </Button>
      {open &&
        createPortal(
          <div
            className="action-menu-popup"
            ref={menu}
            id={id}
            role="menu"
            aria-label={label}
            onBlur={(event) => {
              const target = event.relatedTarget as Node | null;
              if (
                !event.currentTarget.contains(target) &&
                !trigger.current?.contains(target)
              )
                close();
            }}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                event.stopPropagation();
                close(true);
              }
              if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
                event.preventDefault();
                const buttons = [
                  ...event.currentTarget.querySelectorAll<HTMLButtonElement>(
                    "button:not(:disabled)",
                  ),
                ];
                const current = buttons.indexOf(
                  document.activeElement as HTMLButtonElement,
                );
                const next =
                  event.key === "Home"
                    ? 0
                    : event.key === "End"
                      ? buttons.length - 1
                      : (current +
                          (event.key === "ArrowUp" ? -1 : 1) +
                          buttons.length) %
                        buttons.length;
                buttons[next]?.focus();
              }
            }}
          >
            {(groups ?? [{ label: "", items }]).map((group) => (
              <div
                className="action-menu-group"
                key={group.label}
                role="group"
                aria-label={group.label || undefined}
              >
                {group.label && (
                  <span className="action-menu-group-title" aria-hidden="true">
                    {group.label}
                  </span>
                )}
                {group.items.map((item) => (
                  <Button
                    key={item.label}
                    type="button"
                    role={
                      item.checked === undefined
                        ? "menuitem"
                        : "menuitemcheckbox"
                    }
                    aria-checked={item.checked}
                    aria-label={item.label}
                    disabled={item.disabled}
                    data-danger={item.danger}
                    onClick={() => {
                      close(true);
                      item.run();
                    }}
                  >
                    <span className="action-menu-item-label">
                      {item.icon}
                      <span>{item.label}</span>
                    </span>
                    {item.shortcut && <kbd>{item.shortcut}</kbd>}
                  </Button>
                ))}
              </div>
            ))}
            {status && <small role="status">{status}</small>}
          </div>,
          document.body,
        )}
    </div>
  );
}
