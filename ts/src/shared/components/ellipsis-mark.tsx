import styles from "./ellipsis-mark.module.css";

// One text character wide, with the three dots at the line's visual center.
export function EllipsisMark() {
  return (
    <svg
      className={styles.mark}
      viewBox="0 0 6 12"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="0.5" cy="6.5" r="0.5" />
      <circle cx="2.5" cy="6.5" r="0.5" />
      <circle cx="4.5" cy="6.5" r="0.5" />
    </svg>
  );
}
