import { useState, type CSSProperties } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./session-indicator.module.css";

// Keep the braille-like arrangement, ordered around its perimeter.
const activityDots = [
  [4, 2],
  [8, 2],
  [8, 6],
  [8, 10],
  [8, 14],
  [4, 14],
  [4, 10],
  [4, 6],
] as const;
const activityColors = ["green", "cyan", "violet"] as const;

function randomActivityColors() {
  // Keep all toolbar hues present, with a stable random order per indicator.
  const colors = activityDots.map(
    (_, index) => activityColors[index % activityColors.length]!,
  );
  for (let index = colors.length - 1; index > 0; index--) {
    const other = Math.floor(Math.random() * (index + 1));
    [colors[index], colors[other]] = [colors[other]!, colors[index]!];
  }
  return colors;
}

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
  const [dotColors] = useState(randomActivityColors);
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
                  animationDelay: `calc(${index} * var(--session-dot-step) * -1)`,
                  "--session-dot-highlight": `var(--aurora-${dotColors[index]})`,
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
        <rect x="3" y="3" width="10" height="10" rx="1" />
      ) : state === "interrupted" || state === "waiting" ? (
        <path d="M6 4v8m4-8v8" />
      ) : (
        <>
          <circle cx="8" cy="8" r="5.5" />
          {state === "idle" ? (
            <path d="m5.5 8 1.7 1.7 3.5-3.5" />
          ) : state === "failed" ? (
            <path d="M8 4.5V8m0 3h.01" />
          ) : (
            <path d="M6.5 6.25a1.5 1.5 0 1 1 2 1.4c-.5.2-.5.6-.5 1.1M8 11h.01" />
          )}
        </>
      )}
    </svg>
  );
}
