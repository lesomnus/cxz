import type { CSSProperties, ReactNode } from "react";
import styles from "./resource-control-group.module.css";

/** Bound the resource button while its actions fill the remaining control. */
export function ResourceControlGroup({
  children,
  actions,
  leadingWidth,
  label,
}: {
  children: ReactNode;
  actions: ReactNode;
  leadingWidth: CSSProperties["width"];
  label: string;
}) {
  return (
    <div
      data-form-control-group
      role="group"
      aria-label={label}
      className={`resource-control-group ${styles.group}`}
      style={
        {
          "--resource-control-leading-width":
            typeof leadingWidth === "number"
              ? `${leadingWidth}px`
              : leadingWidth,
        } as CSSProperties
      }
    >
      <div className={styles.leading}>{children}</div>
      <div className={styles.actions}>{actions}</div>
    </div>
  );
}
