import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Button } from "./button";

export function ActionMenu({
  label,
  items,
  status,
}: {
  label: string;
  items: {
    label: string;
    shortcut?: string;
    checked?: boolean;
    run: () => void;
  }[];
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
    const bounds = trigger.current!.getBoundingClientRect();
    const popup = menu.current;
    popup.style.top = `${bounds.bottom + 4}px`;
    popup.style.right = `${Math.max(8, window.innerWidth - bounds.right)}px`;
    popup.querySelector("button")?.focus({ preventScroll: true });
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
                  ...event.currentTarget.querySelectorAll("button"),
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
            {items.map((item) => (
              <Button
                key={item.label}
                type="button"
                role={
                  item.checked === undefined ? "menuitem" : "menuitemcheckbox"
                }
                aria-checked={item.checked}
                aria-label={item.label}
                onClick={() => {
                  close(true);
                  item.run();
                }}
              >
                <span>{item.label}</span>
                {item.shortcut && <kbd>{item.shortcut}</kbd>}
              </Button>
            ))}
            {status && <small role="status">{status}</small>}
          </div>,
          document.body,
        )}
    </div>
  );
}
