import { Icon } from "#src/shared/icons/icon.tsx";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "@lesomnus/nox";
import { ErrorPanel } from "#src/shared/errors/error-panel.tsx";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./error-center.module.css";

export function ErrorCenter({
  store = clientErrors(),
}: {
  store?: ClientErrors;
}) {
  useLocale();
  const entries = useSyncExternalStore(
    store.subscribe,
    store.snapshot,
    store.snapshot,
  );
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState("active");
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const id = useId();
  const active = entries.filter(
    (entry) => entry.status === "active" && entry.notify !== false,
  );
  const close = (restore = false) => {
    setOpen(false);
    if (restore) trigger.current?.focus({ preventScroll: true });
  };
  useLayoutEffect(() => {
    if (!open || !popup.current || !trigger.current) return;
    const panel = popup.current;
    const position = () => {
      const anchor = trigger.current!.getBoundingClientRect();
      panel.style.left = `${Math.max(8, Math.min(anchor.right + 4, innerWidth - panel.offsetWidth - 8))}px`;
      panel.style.top = `${Math.max(8, Math.min(anchor.bottom - panel.offsetHeight, innerHeight - panel.offsetHeight - 8))}px`;
    };
    position();
    panel
      .querySelector<HTMLButtonElement>(".card-close")
      ?.focus({ preventScroll: true });
    const resize = new ResizeObserver(position);
    resize.observe(panel);
    resize.observe(trigger.current);
    window.addEventListener("resize", position);
    document.addEventListener("scroll", position, true);
    return () => {
      resize.disconnect();
      window.removeEventListener("resize", position);
      document.removeEventListener("scroll", position, true);
    };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (
        !popup.current?.contains(target) &&
        !trigger.current?.contains(target)
      )
        close();
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [open]);
  return (
    <div className={styles.root}>
      <Button
        ref={trigger}
        type="button"
        effect="none"
        className={`resource-link ${styles.trigger}`}
        aria-label={t("Errors")}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        data-active={active.length > 0 || undefined}
        onClick={() => {
          if (!open) store.restore();
          setOpen(!open);
        }}
      >
        <span className={styles.symbol} aria-hidden="true">
          <Icon name="error-active" width="20" height="20" />
        </span>
        <span className={styles.srOnly} role="status">
          {t("{count} error alerts", { count: String(active.length) })}
        </span>
      </Button>
      {open &&
        createPortal(
          <div
            ref={popup}
            className={styles.popup}
            id={id}
            role="dialog"
            aria-label={t("Errors")}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                event.stopPropagation();
                close(true);
              }
            }}
            onBlur={(event) => {
              const next = event.relatedTarget as Node | null;
              if (
                next &&
                !popup.current?.contains(next) &&
                !trigger.current?.contains(next)
              )
                close();
            }}
          >
            <ErrorPanel
              store={store}
              tab={tab}
              onTabChange={setTab}
              close={() => close(true)}
            />
          </div>,
          document.body,
        )}
    </div>
  );
}
