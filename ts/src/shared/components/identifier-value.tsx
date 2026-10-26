import { CopyButton } from "./copy-button";
import { Popover } from "@lesomnus/nox";
import styles from "./identifier-value.module.css";

export function IdentifierValue({ value }: { value?: string }) {
  if (!value) return <>—</>;
  const prefix = <span className={styles.text}>{value.slice(0, 7)}</span>;
  return (
    <span className={styles.value}>
      {value.length > 7 ? (
        <Popover content={value} placement="in-place">
          {prefix}
        </Popover>
      ) : (
        prefix
      )}
      <CopyButton value={value} />
    </span>
  );
}
