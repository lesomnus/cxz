import { CompactNumber, DialogReveal, DurationDisplay } from "@lesomnus/nox";
import { useEffect, useId, useRef, useState } from "react";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import styles from "./usage-info.module.css";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  quotaDots,
  type SessionInfo,
} from "#src/features/session/model/session-info.ts";

function quotaPeriod(label: string | undefined) {
  const period = label?.split(" · ").at(-1) || "—";
  return (
    (
      {
        "five hour": "5h",
        five_hour: "5h",
        "seven day": "weekly",
        seven_day: "weekly",
        weekly: "weekly",
      } as Record<string, string>
    )[period] ?? period
  );
}

export function UsageInfo({
  info,
  agent,
}: {
  info: SessionInfo;
  agent: string;
}) {
  useLocale();
  const quotaId = useId();
  const contextId = useId();
  const quota = useRef<HTMLSpanElement>(null);
  const context = useRef<HTMLSpanElement>(null);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const open = hovered || focused;
  const [contextHovered, setContextHovered] = useState(false);
  const [contextFocused, setContextFocused] = useState(false);
  const contextOpen = contextHovered || contextFocused;
  useEffect(() => {
    if (!hovered && !contextHovered) return;
    // Revealing or replacing a child under the pointer can lose a delegated
    // mouseleave. Keep hover in sync with the next native pointer movement.
    const move = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      if (hovered && !quota.current?.contains(target)) setHovered(false);
      if (contextHovered && !context.current?.contains(target))
        setContextHovered(false);
    };
    const blur = () => {
      setHovered(false);
      setContextHovered(false);
    };
    document.addEventListener("pointermove", move, true);
    window.addEventListener("blur", blur);
    return () => {
      document.removeEventListener("pointermove", move, true);
      window.removeEventListener("blur", blur);
    };
  }, [hovered, contextHovered]);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!open) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [open]);
  const known = info.contextUsed !== undefined && !!info.contextWindow;
  const ratio = known
    ? Math.min(1, info.contextUsed! / info.contextWindow!)
    : 0;
  return (
    <div className={`usage-info ${styles.usage}`}>
      <span
        ref={quota}
        className={`meta-popover ${styles.quota}`}
        data-open={open}
        data-quota-level={
          info.remaining === undefined
            ? "unknown"
            : info.remaining < 10
              ? "critical"
              : info.remaining <= 30
                ? "warning"
                : "normal"
        }
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        tabIndex={0}
        aria-describedby={quotaId}
      >
        <span
          className={`quota-dots ${styles.graph}`}
          role="img"
          aria-label={
            info.remaining === undefined
              ? t("Remaining quota not reported")
              : t("Remaining quota {percent}%", {
                  percent: Math.round(info.remaining),
                })
          }
        >
          <span>{quotaDots(info.remaining)}</span>
        </span>
        {open && (
          <DialogReveal
            id={quotaId}
            className={styles.details}
            contentClassName={styles.content}
            from="right"
            role="tooltip"
          >
            <AgentBrand agent={agent} />
            <span className={styles.period}>
              {quotaPeriod(info.quotaLabel)}
            </span>
            <span className={styles.time}>
              left{" "}
              <DurationDisplay
                durationMs={
                  info.reset === undefined
                    ? undefined
                    : Math.max(0, info.reset - now)
                }
                rounding="ceil"
              />
            </span>
            <span className={styles.percent}>
              {info.remaining === undefined
                ? "—"
                : `${Math.round(info.remaining)}%`}
            </span>
          </DialogReveal>
        )}
      </span>
      <span
        ref={context}
        className={`meta-popover context-popover ${styles.context}`}
        data-open={contextOpen}
        onMouseEnter={() => setContextHovered(true)}
        onMouseLeave={() => setContextHovered(false)}
        onFocus={() => setContextFocused(true)}
        onBlur={() => setContextFocused(false)}
        tabIndex={0}
        aria-describedby={contextId}
      >
        <svg
          className={`context-donut ${styles.donut}`}
          width="18"
          height="18"
          viewBox="0 0 20 20"
          role="img"
          aria-label={t("Context usage")}
        >
          <circle
            className="context-track"
            cx="10"
            cy="10"
            r="7"
            fill="none"
            strokeWidth="3"
          />
          <circle
            className="context-fill"
            cx="10"
            cy="10"
            r="7"
            fill="none"
            stroke="currentColor"
            strokeWidth="3"
            pathLength="1"
            strokeDasharray={`${ratio} 1`}
            transform="rotate(-90 10 10)"
          />
        </svg>
        {contextOpen && (
          <DialogReveal
            id={contextId}
            className={styles.details}
            contentClassName={styles.contextContent}
            from="right"
            role="tooltip"
          >
            <CompactNumber value={info.contextUsed ?? NaN} />
            <span className={styles.separator}>/</span>
            <CompactNumber value={info.contextWindow ?? NaN} />
            <span className={styles.percent}>
              {known ? `${Math.round(ratio * 100)}%` : "—"}
            </span>
          </DialogReveal>
        )}
      </span>
    </div>
  );
}
