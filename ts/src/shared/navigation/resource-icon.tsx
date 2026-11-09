import { Icon } from "#src/shared/icons/icon.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./resource-icon.module.css";
import { BadgedIcon } from "@lesomnus/nox";

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
    | "tasks"
    | "settings"
    | "logs"
    | "accounts"
    | "search"
    | "memory"
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
      <Icon name={kind} size="100%" />
    </BadgedIcon>
  );
}
