import type { ReactNode } from "react";
import styles from "./dialog-surface.module.css";

// Animate the content, keeping the native dialog's viewport coordinate space
// unchanged for fixed-position menus portaled into its top layer.
export function DialogSurface({
  children,
  anchored = false,
}: {
  children: ReactNode;
  anchored?: boolean;
}) {
  return (
    <div className={styles.surface} data-anchored={anchored}>
      {children}
    </div>
  );
}
