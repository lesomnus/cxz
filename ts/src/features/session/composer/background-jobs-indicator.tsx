import { useId } from "react";
import { Button } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./background-jobs-indicator.module.css";

export function BackgroundJobsIndicator({
  count,
  open,
}: {
  count?: number;
  open?: () => void;
}) {
  useLocale();
  const hint = useId();
  const active = count !== undefined && count > 0;
  const label =
    count === undefined
      ? t("Background job status unavailable")
      : t(
          count === 1
            ? "{count} active background job"
            : "{count} active background jobs",
          { count },
        );
  return (
    <span className="meta-popover">
      <Button
        type="button"
        pressTarget={`.${styles.count}`}
        className={`toolbar-button ${styles.indicator}`}
        aria-label={label}
        aria-describedby={hint}
        data-background-jobs={count ?? "unknown"}
        data-active={active}
        onClick={open}
      >
        <svg className={styles.border} aria-hidden="true">
          <rect className={styles.rail} x="0.5" y="0.5" pathLength="8" />
          <rect
            className={styles.glow}
            x="0.5"
            y="0.5"
            pathLength="8"
            strokeDasharray={`${Math.min(count ?? 0, 8)} ${8 - Math.min(count ?? 0, 8)}`}
          />
        </svg>
        <span className={styles.count}>
          {count === undefined ? "—" : count > 999 ? "999+" : count}
        </span>
      </Button>
      <span id={hint} className="meta-tooltip" role="tooltip">
        {label}
      </span>
    </span>
  );
}
