import { Icon } from "#src/shared/icons/icon.tsx";
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
      <CheckboxMark />
    </label>
  );
}

export function CheckboxMark({
  checked = false,
  indeterminate = false,
}: {
  checked?: boolean;
  indeterminate?: boolean;
}) {
  return (
    <span
      className={styles.box}
      data-checked={checked || undefined}
      data-indeterminate={indeterminate || undefined}
      aria-hidden="true"
    >
      <Icon name="checkbox" />
    </span>
  );
}
