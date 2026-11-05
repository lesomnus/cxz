import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  Button,
  DialogReveal,
  MenuSurface,
  ScrollArea,
  SegmentedControl,
  CompactNumber,
} from "@lesomnus/nox";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
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
  const closeButton = useRef<HTMLButtonElement>(null);
  const id = useId();
  const active = entries.filter((entry) => entry.status === "active");
  const history = entries.filter((entry) => entry.status !== "active");
  const shown = tab === "active" ? active : history;
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
    closeButton.current?.focus({ preventScroll: true });
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
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          >
            <path d="m12 3 10 18H2L12 3Z" strokeLinejoin="round" />
            <path d="M12 9v5m0 3v.1" strokeLinecap="round" />
          </svg>
        </span>
        <span className={styles.srOnly} role="status">
          {t("{count} active errors", { count: String(active.length) })}
        </span>
      </Button>
      {open && (
        <MenuSurface
          ref={popup}
          anchor={trigger.current}
          id={id}
          role="dialog"
          aria-label={t("Errors")}
          className={styles.popup}
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
          <DialogReveal
            from="left"
            className={styles.reveal}
            contentClassName={styles.reveal}
          >
            <header className={styles.header}>
              <strong>{t("Errors")}</strong>
              <SegmentedControl
                label={t("Error status")}
                size="compact"
                value={tab}
                onChange={setTab}
                options={[
                  {
                    value: "active",
                    label: `${t("Active")} ${active.length}`,
                    display: (
                      <>
                        {t("Active")}
                        <CompactNumber
                          value={active.length}
                          mode="integer"
                          digits={3}
                        />
                      </>
                    ),
                  },
                  {
                    value: "history",
                    label: `${t("History")} ${history.length}`,
                    display: (
                      <>
                        {t("History")}
                        <CompactNumber
                          value={history.length}
                          mode="integer"
                          digits={3}
                        />
                      </>
                    ),
                  },
                ]}
              />
              <Button
                ref={closeButton}
                variant="toolbar"
                aria-label={t("Close errors")}
                onClick={() => close(true)}
              >
                ×
              </Button>
            </header>
            <ScrollArea
              className={styles.list}
              aria-label={t("Error list")}
              scrollbarLabel={t("Scroll errors")}
            >
              {!shown.length ? (
                <p className={styles.empty}>
                  {tab === "active"
                    ? t("No active errors")
                    : t("No error history")}
                </p>
              ) : (
                <ul>
                  {shown.map((entry) => (
                    <li key={entry.id} data-status={entry.status}>
                      <div className={styles.row}>
                        <span className={styles.state}>
                          {t(
                            entry.status === "active"
                              ? "Active"
                              : entry.status === "resolved"
                                ? "Resolved"
                                : "Dismissed",
                          )}
                        </span>
                        <span className={styles.time}>
                          <RelativeTime value={entry.lastSeen} />
                        </span>
                        {entry.status === "active" && (
                          <Button
                            className={styles.dismiss}
                            onClick={() => store.dismiss(entry.id)}
                          >
                            {t("Dismiss")}
                          </Button>
                        )}
                      </div>
                      <p className={styles.message}>{entry.message}</p>
                      {entry.method && (
                        <code className={styles.method}>{entry.method}</code>
                      )}
                      {entry.target && (
                        <code className={styles.method}>{entry.target}</code>
                      )}
                      <div className={styles.details}>
                        {entry.code && <span>{entry.code}</span>}
                        {entry.count > 1 && (
                          <span>
                            <CompactNumber
                              value={entry.count}
                              mode="integer"
                              digits={3}
                            />{" "}
                            {t("occurrences")}
                          </span>
                        )}
                        {entry.connection && <span>{entry.connection}</span>}
                      </div>
                      <div className={styles.details}>
                        <span>
                          {t("First seen")}:{" "}
                          <DateTime value={entry.firstSeen} />
                        </span>
                        {entry.closedAt && (
                          <span>
                            {t(
                              entry.status === "resolved"
                                ? "Resolved"
                                : "Dismissed",
                            )}
                            : <DateTime value={entry.closedAt} />
                          </span>
                        )}
                      </div>
                      {(entry.session || entry.project) && (
                        <code className={styles.method}>
                          {entry.session
                            ? `Session ${entry.session}`
                            : `Project ${entry.project}`}
                        </code>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </ScrollArea>
            <footer className={styles.footer}>
              <Button
                disabled={!history.length}
                onClick={() => store.clearHistory()}
              >
                {t("Clear history")}
              </Button>
            </footer>
          </DialogReveal>
        </MenuSurface>
      )}
    </div>
  );
}
