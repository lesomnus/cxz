import { CopyButton } from "./copy-button";
import { OverflowHint } from "@lesomnus/nox";
import styles from "./identifier-value.module.css";

export function IdentifierValue({ value }: { value?: string }) {
  if (!value) return <>—</>;
  const prefix = <span className={styles.text}>{value.slice(0, 7)}</span>;
  return (
    <span className={styles.value}>
      {value.length > 7 ? (
        <OverflowHint content={value}>{prefix}</OverflowHint>
      ) : (
        prefix
      )}
      <CopyButton value={value} />
    </span>
  );
}
