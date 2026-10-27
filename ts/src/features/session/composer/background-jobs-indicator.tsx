import { useId } from "react";
import { BorderTrailButton } from "@lesomnus/nox";
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
      <BorderTrailButton
        type="button"
        active={active}
        progress={{ value: count ?? 0, perSegment: 8 }}
        sheen
        className={`toolbar-button ${styles.indicator}`}
        aria-label={label}
        aria-describedby={hint}
        data-background-jobs={count ?? "unknown"}
        data-active={active}
        onClick={open}
      >
        <span className={styles.count}>
          {count === undefined ? "—" : count > 999 ? "999+" : count}
        </span>
      </BorderTrailButton>
      <span id={hint} className="meta-tooltip" role="tooltip">
        {label}
      </span>
    </span>
  );
}
