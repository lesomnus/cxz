import { Popover } from "@lesomnus/nox";
import type { LogLevel } from "#src/shared/logs/log-entry.ts";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import styles from "./logs-page.module.css";

const icons: Record<LogLevel, IconName> = {
  info: "log-level-info",
  warn: "log-level-warn",
  error: "log-level-error",
};
export function LogLevelIcon({ level }: { level: LogLevel }) {
  return (
    <Popover content={level}>
      <span
        className={styles.level}
        data-level={level}
        role="img"
        aria-label={level}
      >
        <Icon name={icons[level]} size={14} />
      </span>
    </Popover>
  );
}
