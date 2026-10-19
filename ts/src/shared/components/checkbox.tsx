import type { InputHTMLAttributes } from "react";
import styles from "./checkbox.module.css";

export function Checkbox({
  className = "",
  indeterminate = false,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "children"> & {
  indeterminate?: boolean;
}) {
  return (
    <label className={`${styles.control} ${className}`}>
      <input
        {...props}
        className={styles.input}
        type="checkbox"
        ref={(node) => {
          if (node) node.indeterminate = indeterminate;
        }}
      />
      <span className={styles.box} aria-hidden="true">
        <svg
          viewBox="0 0 18 18"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path className={styles.check} d="m4.5 9 3 3 6-6" />
          <path className={styles.mixed} d="M5 9h8" />
        </svg>
      </span>
    </label>
  );
}
