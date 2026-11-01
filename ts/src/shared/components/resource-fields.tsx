import type { CSSProperties, ReactNode } from "react";
import { FieldLabel } from "./field-label";
import styles from "./resource-fields.module.css";

export type ResourceFieldRow = readonly [
  label: string,
  value: ReactNode,
  description?: string,
];

// Keep labels, values, help and field spacing consistent across resource pages.
export function ResourceFields({
  rows,
  columns = 2,
}: {
  rows: readonly ResourceFieldRow[];
  columns?: 1 | 2;
}) {
  return (
    <dl
      className={styles.fields}
      style={{ "--resource-field-columns": columns } as CSSProperties}
    >
      {rows.map(([label, value, description]) => (
        <div key={label}>
          <dt>
            <FieldLabel label={label} description={description} />
          </dt>
          <dd>
            {value === undefined || value === null || value === ""
              ? "—"
              : value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
