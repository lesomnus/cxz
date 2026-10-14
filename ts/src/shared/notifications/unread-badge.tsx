import type { CSSProperties } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { SESSION_ATTENTION_COLOR } from "./attention-color";
import { SESSION_ERROR_COLOR } from "./error-color";
import styles from "./unread-badge.module.css";

export function UnreadBadge({
  count,
  error = false,
}: {
  count?: number;
  error?: boolean;
}) {
  useLocale();
  return (
    <span
      className={`${styles.badge} ${count === undefined ? "" : styles.count}`}
      style={
        {
          "--attention-color": error
            ? SESSION_ERROR_COLOR
            : SESSION_ATTENTION_COLOR,
        } as CSSProperties
      }
      role="img"
      aria-label={
        error
          ? t("Unread error")
          : count === undefined
            ? t("Unread response")
            : t("{count} unread sessions", { count })
      }
    >
      {count === undefined ? (
        <svg width="6" height="6" viewBox="0 0 6 6" aria-hidden="true">
          <circle cx="3" cy="3" r="3" fill="currentColor" />
        </svg>
      ) : (
        <span aria-hidden="true">{count > 99 ? "99+" : count}</span>
      )}
    </span>
  );
}
