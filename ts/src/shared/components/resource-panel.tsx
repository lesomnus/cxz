import type { ComponentProps, ReactNode } from "react";
import { PanelScroll } from "./panel-scroll";
import styles from "./resource-panel.module.css";

// Persistent resource navigation uses the same distinct header and rounded
// inset body as Card, without modal behavior or floating-card effects.
export function ResourcePanel({
  title,
  actions,
  children,
  className = "",
  unreadSelector,
  ...props
}: Omit<ComponentProps<"aside">, "title"> & {
  title: ReactNode;
  actions?: ReactNode;
  unreadSelector?: string;
}) {
  return (
    <aside {...props} className={`resource-panel ${styles.panel} ${className}`}>
      <header className={styles.header}>
        <strong className={styles.title}>{title}</strong>
        {actions && <div className={styles.actions}>{actions}</div>}
      </header>
      <PanelScroll unreadSelector={unreadSelector}>{children}</PanelScroll>
    </aside>
  );
}
