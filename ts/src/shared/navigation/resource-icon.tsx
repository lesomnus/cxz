import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./resource-icon.module.css";
import { BadgedIcon } from "@lesomnus/cxz-ui";

export function ResourceIcon({
  kind,
  unreadCount = 0,
  error = false,
}: {
  unreadCount?: number;
  error?: boolean;
  kind:
    | "sessions"
    | "projects"
    | "settings"
    | "accounts"
    | "search"
    | "memory"
    | "activity"
    | "auxiliary";
}) {
  useLocale();
  return (
    <BadgedIcon
      className={styles.icon}
      badgeClassName={styles.badge}
      badge={
        unreadCount > 0 ? (
          <UnreadBadge count={unreadCount} error={error} />
        ) : undefined
      }
    >
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {kind === "sessions" ? (
          <path d="M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z" />
        ) : kind === "projects" ? (
          <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
        ) : kind === "accounts" ? (
          <>
            <circle cx="12" cy="8" r="4" />
            <path d="M4 21v-3a8 8 0 0 1 16 0v3" />
          </>
        ) : kind === "search" ? (
          <>
            <circle cx="10" cy="10" r="6" />
            <path d="m15 15 6 6" />
          </>
        ) : kind === "memory" ? (
          <>
            <path d="M4 3h16v18H4Z M8 7h8M8 11h8M8 15h5" />
          </>
        ) : kind === "activity" ? (
          <path d="M3 12h4l3-8 4 16 3-8h4" />
        ) : kind === "auxiliary" ? (
          <>
            <path d="m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z" />
          </>
        ) : (
          <>
            <circle cx="12" cy="12" r="3" />
            <path d="M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1Z" />
          </>
        )}
      </svg>
    </BadgedIcon>
  );
}
