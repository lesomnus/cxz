import { useCallback, useId, useState, type ReactNode, type Ref } from "react";
import { AnchoredMenu } from "#src/shared/components/anchored-menu.tsx";
import { BorderTrailButton } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./background-jobs-indicator.module.css";

export function BackgroundJobsIndicator({
  count,
  open,
  report,
  buttonRef,
}: {
  count?: number;
  open?: () => void;
  report?: ReactNode;
  buttonRef?: Ref<HTMLButtonElement>;
}) {
  useLocale();
  const hint = useId();
  const menuId = useId();
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const close = useCallback(
    (restoreFocus = false) => {
      setAnchor(null);
      if (restoreFocus) anchor?.focus({ preventScroll: true });
    },
    [anchor],
  );
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
        ref={buttonRef}
        type="button"
        active={active}
        progress={{ value: count ?? 0, perSegment: 8 }}
        sheen
        className={`toolbar-button ${styles.indicator}`}
        aria-label={label}
        aria-describedby={anchor ? undefined : hint}
        aria-haspopup={report ? "menu" : undefined}
        aria-expanded={report ? !!anchor : undefined}
        aria-controls={anchor ? menuId : undefined}
        data-background-jobs={count ?? "unknown"}
        data-active={active}
        onClick={(event) => {
          if (report) {
            const button = event.currentTarget;
            setAnchor((current) => (current ? null : button));
          } else open?.();
        }}
      >
        <span className={styles.count}>
          {count === undefined ? "—" : count > 999 ? "999+" : count}
        </span>
      </BorderTrailButton>
      {!anchor && (
        <span id={hint} className="meta-tooltip" role="tooltip">
          {label}
        </span>
      )}
      {anchor && report && (
        <AnchoredMenu
          id={menuId}
          anchor={anchor}
          label={t("Background tasks")}
          close={close}
        >
          {report}
        </AnchoredMenu>
      )}
    </span>
  );
}
