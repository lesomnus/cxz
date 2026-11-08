import type { ComponentProps, ReactNode } from "react";
import styles from "./menu-header.module.css";

/** MenuSurface owns the outer inset and exposes its concentric inner corners. */
export function MenuHeader({
  children,
  actions,
  className = "",
  ...props
}: ComponentProps<"header"> & { actions?: ReactNode }) {
  return (
    <header {...props} className={`${styles.header} ${className}`}>
      <strong className={styles.title}>{children}</strong>
      {actions && <div className={styles.actions}>{actions}</div>}
    </header>
  );
}
