import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { Button } from "./button";
import type { Session, SessionEvent } from "../gen/cxz/session_pb";
import { payload } from "./journal";
import type { SessionInfo } from "./session-info";

type ModelOption = {
  id: string;
  resolved_id?: string;
  name?: string;
  efforts?: string[];
  default_effort?: string;
  default?: boolean;
};
export type ModelCatalog = {
  models: ModelOption[];
  model?: string;
  effort?: string;
  effective_model?: string;
};
export function modelCatalog(
  s: Session | undefined,
  events: SessionEvent[],
): ModelCatalog | undefined {
  const event = [...events]
    .reverse()
    .find((e) => e.kind === "models" && e.runId === s?.status?.runId);
  if (!event) return;
  const p = payload(event);
  if (!Array.isArray(p.models)) return;
  return {
    ...p,
    models: p.models.filter(
      (m: ModelOption) => m && typeof m.id === "string" && /^\S+$/.test(m.id),
    ),
  };
}
export function selectedModel(c: ModelCatalog) {
  const applied = c.effective_model || c.model;
  const exact = c.models.find(
    (m) =>
      m.id === c.model &&
      (!applied || m.id === applied || m.resolved_id === applied),
  );
  if (exact) return exact;
  if (applied && applied !== "default")
    return (
      c.models.find((m) => m.id === applied || m.resolved_id === applied) ||
      c.models.find((m) => m.id === c.model && !m.resolved_id)
    );
  return c.models.find((m) => m.default || m.id === "default");
}
export function ModelSettings({
  session,
  info,
  catalog,
  busy,
  change,
}: {
  session?: Session;
  info: SessionInfo;
  catalog?: ModelCatalog;
  busy: boolean;
  change: (kind: "model" | "effort", value: string) => void;
}) {
  const selected = catalog && selectedModel(catalog);
  const models = catalog?.models.map((m) => m.id) ?? [];
  if (catalog?.models.some((m) => m.default) && !models.includes("default"))
    models.unshift("default");
  const efforts = (selected?.efforts || []).filter(
    (v) => typeof v === "string" && /^\S+$/.test(v),
  );
  if (
    efforts.length &&
    (session?.agent === "claude" || selected?.default_effort)
  )
    efforts.push("default");
  return (
    <div className="model-info">
      {(["model", "effort"] as const).map((kind) => {
        const label = kind === "model" ? "Model" : "Effort";
        const values = [...new Set(kind === "model" ? models : efforts)];
        const value =
          (kind === "model" ? catalog?.model : catalog?.effort) || "default";
        const disabled =
          busy || session?.status?.state !== "idle" || !values.length;
        return (
          <div key={kind} className={`${kind}-field setting-field`}>
            <span className="meta-label">{label}</span>
            <ValueMenu
              label={label}
              display={info[kind] || "—"}
              value={value}
              values={values}
              disabled={disabled}
              disabledReason={
                !values.length
                  ? "Provider choices not reported"
                  : "Settings require an idle session"
              }
              choose={(choice) => change(kind, choice)}
            />
          </div>
        );
      })}
    </div>
  );
}

function ValueMenu({
  label,
  display,
  value,
  values,
  disabled,
  disabledReason,
  choose,
}: {
  label: string;
  display: string;
  value: string;
  values: string[];
  disabled: boolean;
  disabledReason: string;
  choose: (value: string) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const id = useId();
  const [open, setOpen] = useState(false);
  const [up, setUp] = useState(true);
  const options = values.filter((v) => v !== value && v !== display);
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
    const rect = root.current.getBoundingClientRect();
    const height = 20 + 16 + 13 + options.length * 28;
    const upwards = rect.top + height > window.innerHeight;
    setUp(upwards);
    menu.current.style.setProperty(
      "--menu-width",
      `${Math.max(rect.width + 16, Math.min(label === "Model" ? 240 : 140, window.innerWidth - rect.left))}px`,
    );
    menu.current.style.setProperty(
      "--options-height",
      `${Math.max(28, Math.min(240, (upwards ? rect.bottom : window.innerHeight - rect.top) - 49))}px`,
    );
    menu.current
      .querySelector<HTMLButtonElement>("[role=option]")
      ?.focus({ preventScroll: true });
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const outside = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) close();
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
      onKeyDown={key}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) close();
      }}
    >
      <Button
        type="button"
        className="setting-trigger"
        role="combobox"
        aria-label={label}
        aria-haspopup="listbox"
        aria-controls={id}
        aria-expanded={open}
        disabled={disabled}
        title={disabled ? disabledReason : display}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="meta-value">{display}</span>
      </Button>
      {open && (
        <div
          ref={menu}
          id={id}
          className={`setting-menu ${up ? "opens-up" : ""}`}
          role="listbox"
          aria-label={`${label} choices`}
        >
          <Button
            type="button"
            className="setting-current"
            role="option"
            tabIndex={-1}
            aria-selected="true"
            onClick={() => close(true)}
          >
            <span className="meta-value">{display}</span>
          </Button>
          <div className="setting-divider" role="separator" />
          <div className="setting-options">
            {options.map((choice) => (
              <Button
                key={choice}
                type="button"
                role="option"
                tabIndex={-1}
                aria-selected="false"
                title={choice}
                onClick={() => {
                  close(true);
                  choose(choice);
                }}
              >
                {choice}
              </Button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
