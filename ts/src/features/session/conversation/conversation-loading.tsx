import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./conversation-loading.module.css";

export type ConversationLoad = {
  stage: "session" | "history" | "retrying";
  error?: string;
};

export function ConversationLoading({ stage, error }: ConversationLoad) {
  useLocale();
  const label =
    stage === "session"
      ? t("Loading session…")
      : stage === "history"
        ? t("Loading conversation…")
        : t("Retrying conversation…");
  return (
    <div className={styles.overlay}>
      <div
        className={styles.status}
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        <svg className={styles.spinner} viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="9" />
          <path d="M12 3a9 9 0 0 1 9 9" />
        </svg>
        <span>{label}</span>
        {error && <small className={styles.error}>{error}</small>}
      </div>
    </div>
  );
}
