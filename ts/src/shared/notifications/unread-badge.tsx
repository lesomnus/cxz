import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { SESSION_ATTENTION_COLOR } from "./attention-color";
import styles from "./unread-badge.module.css";

export function UnreadBadge({ count }: { count?: number }) {
  useLocale();
  return (
    <span
      className={styles.badge}
      style={{ color: SESSION_ATTENTION_COLOR }}
      role="img"
      aria-label={
        count === undefined
          ? t("Unread response")
          : t("{count} unread sessions", { count })
      }
    >
      <svg width="8" height="8" viewBox="0 0 8 8" aria-hidden="true">
        <circle cx="4" cy="4" r="4" fill="currentColor" />
      </svg>
      {count !== undefined && <span aria-hidden="true">{count}</span>}
    </span>
  );
}
