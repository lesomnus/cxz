import {
  Section,
  LiquidGauge,
  AnimatedCompactNumber,
  AnimatedDurationDisplay,
} from "@lesomnus/nox";
import { useEffect, useState } from "react";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useAccountQuota } from "./use-account-quota";
import styles from "./accounts.module.css";

export function AccountQuotaSquare({
  c,
  account,
  agent,
  sessions,
  identity = account,
  refreshVersion = 0,
  onLoadingChange,
  inline = false,
  className = "",
  gaugeClassName = "",
}: {
  c: Connection;
  account: string;
  agent: string;
  sessions: Session[];
  identity?: string;
  refreshVersion?: number;
  onLoadingChange?: (loading: boolean) => void;
  /** Phrasing content for account selectors, retaining the same gauge/countdown. */
  inline?: boolean;
  className?: string;
  gaugeClassName?: string;
}) {
  const quota = useAccountQuota({
    c,
    agent,
    sessions,
    identity,
    refreshVersion,
  });
  useEffect(
    () => onLoadingChange?.(quota.loading),
    [onLoadingChange, quota.loading],
  );
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    setNow(Date.now());
    if (quota.data?.reset === undefined) return;
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") setNow(Date.now());
    }, 1000);
    return () => clearInterval(timer);
  }, [quota.data?.reset]);
  const remaining = quota.data?.remaining;
  const percent = remaining === undefined ? undefined : Math.round(remaining);
  const level =
    remaining === undefined
      ? "unknown"
      : remaining < 10
        ? "critical"
        : remaining <= 30
          ? "warning"
          : "normal";
  const Surface = inline ? "span" : Section;
  return (
    <Surface
      className={`${styles.quotaSquare} ${inline ? styles.quotaInline : ""} ${className}`}
      aria-label="Remaining agent usage"
      data-quota-level={level}
    >
      <LiquidGauge
        className={`${styles.quotaGauge} ${gaugeClassName}`}
        value={remaining}
        aria-label={
          percent === undefined
            ? quota.error
              ? "Usage unavailable"
              : "Not reported"
            : "Remaining agent usage"
        }
        aria-valuetext={
          percent === undefined ? undefined : `${percent}% remaining`
        }
      >
        <AnimatedCompactNumber
          mode="integer"
          digits={percent === 100 ? 3 : 2}
          value={percent ?? NaN}
          muted={percent === undefined}
          animate={!quota.fromCache}
        />
        <span className={styles.quotaUnit}>%</span>
      </LiquidGauge>
      <span className={styles.quotaReset}>
        <AnimatedDurationDisplay
          animate={!quota.fromCache}
          durationMs={
            quota.data?.reset === undefined
              ? undefined
              : Math.max(0, quota.data.reset - now)
          }
          rounding="ceil"
        />
      </span>
    </Surface>
  );
}
