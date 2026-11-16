import { Icon, type ProjectIconName } from "#src/shared/icons/icon.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { UnreadBadge } from "#src/shared/notifications/unread-badge.tsx";
import styles from "./resource-icon.module.css";
import { BadgedIcon } from "@lesomnus/nox";

export function ResourceIcon({
  kind,
  icon,
  unreadCount = 0,
  error = false,
}: {
  unreadCount?: number;
  error?: boolean;
  // A project may stand for itself with a glyph a person chose. Everything
  // else about the icon -- size, badge, alignment -- stays what the kind gets,
  // so a chosen icon cannot drift out of line with the rest of a row.
  icon?: ProjectIconName;
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
      <Icon name={icon ?? kind} size="100%" />
    </BadgedIcon>
  );
}
