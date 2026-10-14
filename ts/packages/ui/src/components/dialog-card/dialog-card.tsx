import type { ComponentProps, ReactNode } from "react";
import { FloatingCard } from "../floating-card/floating-card";
import styles from "./dialog-card.module.css";

// The same header, distinct inset body and compact footer for forms and confirmations.
export function DialogCard({
  className = "",
  ...props
}: ComponentProps<typeof FloatingCard>) {
  return (
    <FloatingCard
      data-entered="true"
      {...props}
      className={`${styles.card} ${className}`}
    />
  );
}

export function DialogActions({ children }: { children: ReactNode }) {
  return <div className={styles.actions}>{children}</div>;
}
