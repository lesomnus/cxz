import type { CSSProperties } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./session-indicator.module.css";
import { activityDots, sessionActivity } from "./session-activity";

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
  const working = ["running", "working", "waiting_input"].includes(state);
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
  const activity = sessionActivity(session);
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
        <svg
          key={activity.seed}
          className={
            !working && background ? styles.backgroundActivity : undefined
          }
          width="12"
          height="16"
          viewBox="0 0 12 16"
          aria-hidden="true"
        >
          {activityDots.map(([cx, cy], index) => (
            <circle
              className="session-activity-dot"
              key={index}
              cx={cx}
              cy={cy}
              r="1.3"
              style={
                {
                  animationDelay: `calc((${index} + ${activity.phase}) * var(--session-dot-step) * -1)`,
                  "--session-dot-highlight": `var(--aurora-${activity.colors[index]})`,
                } as CSSProperties
              }
            />
          ))}
        </svg>
      ) : variant === "status" ? (
        <StateIcon state={state} />
      ) : (
        " "
      )}
    </span>
  );
}

function StateIcon({ state }: { state: string }) {
  return (
    <svg
      className={styles.stateIcon}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {state === "stopped" ? (
        <path d="M2.83 9.88a5.5 5.5 0 0 1 7.05-7.05m3.29 3.29a5.5 5.5 0 0 1-7.05 7.05M2.5 13.5l11-11" />
      ) : state === "interrupted" ? (
        <path d="m8 2.5 5.5 11h-11Z" />
      ) : state === "waiting" ? (
        <path d="M6 4v8m4-8v8" />
      ) : (
        <>
          <circle cx="8" cy="8" r="5.5" />
          {state === "failed" ? (
            <path d="M8 4.5V8m0 3h.01" />
          ) : state !== "idle" ? (
            <path d="M6.5 6.25a1.5 1.5 0 1 1 2 1.4c-.5.2-.5.6-.5 1.1M8 11h.01" />
          ) : null}
        </>
      )}
    </svg>
  );
}
