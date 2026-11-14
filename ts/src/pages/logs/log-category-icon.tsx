import { Popover } from "@lesomnus/nox";
import type { LogCategory } from "#src/shared/logs/log-entry.ts";
import { Icon } from "#src/shared/icons/icon.tsx";
import styles from "./logs-page.module.css";

export function LogCategoryIcon({ category }: { category: LogCategory }) {
  const label = category === "rpc" ? "RPC" : "Client";
  return (
    <Popover content={label}>
      <span className={styles.category} role="img" aria-label={label}>
        <Icon
          name={category === "rpc" ? "log-category-rpc" : "log-category-client"}
          size={14}
        />
      </span>
    </Popover>
  );
}
