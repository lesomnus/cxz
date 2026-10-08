import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "./button";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";

export type ValueOption = { value: string; label: string; muted?: boolean };

// The selected row overlays the trigger text exactly. A portal avoids clipping
// inside settings panes while sharing the composer menu's focus and geometry.
export function ValueMenu({
  label,
  display,
  value,
  options: items,
  muted = false,
  minMenuWidth = 140,
  variant = "default",
  disabled,
  disabledReason,
  choose,
}: {
  label: string;
  display?: string;
  value: string;
  options: ValueOption[];
  muted?: boolean;
  minMenuWidth?: number;
  variant?: "default" | "compact";
  disabled?: boolean;
  disabledReason?: string;
  choose: (value: string) => void;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const id = useId();
  const [open, setOpen] = useState(false);
  const [up, setUp] = useState(true);
  const selected = items.find((item) => item.value === value);
  const text = display ?? selected?.label ?? value;
  const isMuted = muted || selected?.muted;
  const options = items.filter((item) => item.value !== value);
  const trigger = () =>
    root.current?.querySelector<HTMLButtonElement>(".setting-trigger");
  function close(restore = false) {
    setOpen(false);
    if (restore) trigger()?.focus({ preventScroll: true });
  }
  useLayoutEffect(() => {
    const el = root.current;
    if (!el) return;
    const measure = () =>
      el.style.setProperty("--value-width", `${el.clientWidth}px`);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    if (!open || !root.current || !menu.current) return;
    const position = () => {
      if (!root.current || !menu.current) return;
      const rect = root.current.getBoundingClientRect();
      const inset = parseFloat(
        getComputedStyle(root.current).getPropertyValue("--control-inset"),
      );
      const chrome = rect.height + 2 * inset + 13;
      const height = chrome + options.length * rect.height;
      const upwards = rect.top + height > window.innerHeight;
      setUp(upwards);
      const popup = menu.current;
      popup.style.fontSize = getComputedStyle(root.current).fontSize;
      popup.style.setProperty("--value-width", `${rect.width}px`);
      popup.style.setProperty(
        "--menu-width",
        `${Math.min(window.innerWidth - 2 * inset, Math.max(rect.width + 2 * inset, minMenuWidth))}px`,
      );
      popup.style.left = `${Math.min(rect.left - inset, window.innerWidth - parseFloat(popup.style.getPropertyValue("--menu-width")) - inset)}px`;
      popup.style.top = upwards ? "auto" : `${rect.top - inset}px`;
      popup.style.bottom = upwards
        ? `${window.innerHeight - rect.bottom - inset}px`
        : "auto";
      popup.style.setProperty(
        "--options-height",
        `${Math.max(rect.height, Math.min(240, (upwards ? rect.bottom : window.innerHeight - rect.top) - chrome))}px`,
      );
    };
    position();
    const scrolled = (e: Event) => {
      if (!menu.current?.contains(e.target as Node)) position();
    };
    document.addEventListener("scroll", scrolled, true);
    menu.current
      .querySelector<HTMLButtonElement>("[role=option]")
      ?.focus({ preventScroll: true });
    return () => document.removeEventListener("scroll", scrolled, true);
  }, [open, options.length, minMenuWidth]);
  useEffect(() => {
    if (!open) return;
    const outside = (e: PointerEvent) => {
      if (
        !root.current?.contains(e.target as Node) &&
        !menu.current?.contains(e.target as Node)
      )
        close();
    };
    const resized = () => close();
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", resized);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", resized);
    };
  }, [open]);
  useEffect(() => {
    if (disabled) close();
  }, [disabled]);
  function key(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      close(true);
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
    e.preventDefault();
    if (!open) {
      setOpen(true);
      return;
    }
    const buttons = [
      ...(menu.current?.querySelectorAll<HTMLButtonElement>("[role=option]") ||
        []),
    ];
    const visual = up ? [...buttons.slice(1), buttons[0]] : buttons;
    const index = visual.indexOf(document.activeElement as HTMLButtonElement);
    const next =
      e.key === "Home"
        ? 0
        : e.key === "End"
          ? visual.length - 1
          : (index + (e.key === "ArrowUp" ? -1 : 1) + visual.length) %
            visual.length;
    visual[next]?.focus({ preventScroll: true });
  }
  return (
    <div
      ref={root}
      className="value-menu"
      data-variant={variant}
      data-value={value}
      onKeyDown={key}
      onBlur={(e) => {
        if (
          !e.currentTarget.contains(e.relatedTarget as Node | null) &&
          !menu.current?.contains(e.relatedTarget as Node | null)
        )
          close();
      }}
    >
      <Button
        type="button"
        className="setting-trigger"
        data-muted={!!isMuted}
        role="combobox"
        aria-label={label}
        aria-haspopup="listbox"
        aria-controls={id}
        aria-expanded={open}
        disabled={disabled}
        title={disabled ? disabledReason : text}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="meta-value">{text}</span>
      </Button>
      {open &&
        createPortal(
          <div
            ref={menu}
            id={id}
            data-variant={variant}
            className={`setting-menu ${up ? "opens-up" : ""}`}
            onKeyDown={(e) => {
              e.stopPropagation();
              key(e);
            }}
            onBlur={(e) => {
              if (
                !root.current?.contains(e.relatedTarget as Node | null) &&
                !e.currentTarget.contains(e.relatedTarget as Node | null)
              )
                close();
            }}
            role="listbox"
            aria-label={t("{label} choices", { label })}
          >
            <Button
              type="button"
              className="setting-current"
              data-option-value={value}
              data-muted={!!isMuted}
              role="option"
              tabIndex={-1}
              aria-selected="true"
              onClick={() => close(true)}
            >
              <span className="meta-value">{text}</span>
            </Button>
            <div className="setting-divider" role="separator" />
            <div className="setting-options">
              {options.map((choice) => (
                <Button
                  key={choice.value}
                  data-option-value={choice.value}
                  type="button"
                  role="option"
                  tabIndex={-1}
                  aria-selected="false"
                  title={choice.label}
                  data-muted={!!choice.muted}
                  onClick={() => {
                    close(true);
                    choose(choice.value);
                  }}
                >
                  {choice.label}
                </Button>
              ))}
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
