import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./session-indicator.module.css";
import { isSessionWorking } from "./session-activity";
import { SessionActivityDots } from "./session-activity-dots";

export function SessionIndicator({
  session,
  unread = false,
  error = false,
  background = false,
  variant = "activity",
}: {
  session: Session;
  unread?: boolean;
  error?: boolean;
  background?: boolean;
  variant?: "activity" | "status";
}) {
  useLocale();
  const state = session.status?.state || "unknown";
  if (error)
    return (
      <span className="session-indicator" title={t("Unread error")}>
        <UnreadBadge error />
      </span>
    );
  if (variant === "activity" && resumableStates.has(state))
    return <span className="session-indicator" aria-hidden="true" />;
  const question =
    !resumableStates.has(state) && !!session.status?.pending.length;
  const working = isSessionWorking(session);
  const label =
    variant === "activity"
      ? state
      : state === "idle"
        ? t("Idle")
        : state === "stopped"
          ? t("Stopped")
          : state === "interrupted"
            ? t("Interrupted")
            : state === "failed"
              ? t("Failed")
              : state === "waiting" || state === "waiting_input"
                ? t("Waiting for input")
                : working
                  ? t("Working")
                  : t("Unknown");
  if (unread)
    return (
      <span className="session-indicator" title={t("Unread response")}>
        <UnreadBadge />
      </span>
    );
  return (
    <span
      className={`session-indicator ${question ? "attention" : ""}`}
      role="img"
      title={
        variant === "status"
          ? question
            ? t("Awaiting answer")
            : label
          : undefined
      }
      aria-label={
        question
          ? t("Awaiting answer")
          : !working && background
            ? t("Background tasks")
            : label
      }
    >
      {question ? (
        "?"
      ) : working || background ? (
        <SessionActivityDots
          session={session}
          className={
            !working && background ? styles.backgroundActivity : undefined
          }
        />
      ) : variant === "status" ? (
        <StateIcon state={state} />
      ) : (
        " "
      )}
    </span>
  );
}

const stateIcons: Record<string, IconName> = {
  stopped: "state-stopped",
  interrupted: "interrupted",
  waiting: "waiting",
  failed: "failed",
  idle: "idle",
};

function StateIcon({ state }: { state: string }) {
  return (
    <Icon name={stateIcons[state] ?? "unknown"} className={styles.stateIcon} />
  );
}
