import type { ReactNode } from "react";
import styles from "./dialog-surface.module.css";

export const dialogClassName = styles.dialog;
export const dialogAppearanceClassName = styles.appearance;

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
    <div
      className={`${styles.surface} ${styles.appearance}`}
      data-anchored={anchored}
    >
      {children}
    </div>
  );
}
