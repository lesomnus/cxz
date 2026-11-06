import { useEffect, useState, type CSSProperties, type ReactNode } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { FieldLabel } from "./field-label";
import { IdentifierValue } from "./identifier-value";
import { PathValue } from "./path-value";
import styles from "./resource-fields.module.css";

type ResourceField = {
  label: string;
  value: ReactNode;
  description?: string;
  labelDetail?: ReactNode;
  fullWidth?: boolean;
  path?: boolean;
  identifier?: boolean;
  identifierLength?: number;
};

export type ResourceFieldRow =
  | ResourceField
  | readonly [
      label: string,
      value: ReactNode,
      description?: string,
      labelDetail?: ReactNode,
    ];

// Keep labels, values, help and field spacing consistent across resource pages.
export function ResourceFields({
  rows,
  columns = 2,
}: {
  rows: readonly ResourceFieldRow[];
  columns?: 1 | 2 | 3;
}) {
  return (
    <dl
      className={styles.fields}
      style={{ "--resource-field-columns": columns } as CSSProperties}
    >
      {rows.map((row) => {
        const field: ResourceField =
          "label" in row
            ? row
            : {
                label: row[0],
                value: row[1],
                description: row[2],
                labelDetail: row[3],
              };
        return (
          <Field
            key={
              field.path || field.identifier
                ? `${field.label}:${field.value}`
                : field.label
            }
            {...field}
          />
        );
      })}
    </dl>
  );
}

function Field({
  label,
  value,
  description,
  labelDetail,
  fullWidth,
  path,
  identifier,
  identifierLength,
}: ResourceField) {
  useLocale();
  const [feedback, setFeedback] = useState<{
    message: string;
    failed?: boolean;
  }>();
  const [copying, setCopying] = useState(false);
  useEffect(() => {
    if (!feedback) return;
    const timer = setTimeout(() => setFeedback(undefined), 2000);
    return () => clearTimeout(timer);
  }, [feedback]);
  const copyValue =
    (path || identifier) && typeof value === "string" && value
      ? value
      : undefined;
  async function copy() {
    if (!copyValue || copying) return;
    setCopying(true);
    try {
      await navigator.clipboard.writeText(copyValue);
      setFeedback({ message: "copied!" });
    } catch {
      setFeedback({ message: t("Copy failed"), failed: true });
    } finally {
      setCopying(false);
    }
  }
  return (
    <div className={fullWidth ? styles.fullWidth : undefined}>
      <dt>
        <FieldLabel label={label} description={description} />
        {labelDetail != null && (
          <span className={styles.labelDetail}>{labelDetail}</span>
        )}
        {(path || identifier) && (
          <span
            className={feedback?.failed ? styles.copyError : styles.labelDetail}
            role="status"
          >
            {feedback?.message}
          </span>
        )}
      </dt>
      <dd>
        {copyValue && identifier ? (
          <IdentifierValue
            value={copyValue}
            prefixLength={identifierLength}
            label={`${t("Copy")}: ${label}`}
            copying={copying}
            onCopy={copy}
          />
        ) : copyValue && path ? (
          <PathValue
            value={copyValue}
            label={`${t("Copy")}: ${label}`}
            copying={copying}
            onCopy={copy}
          />
        ) : value === undefined || value === null || value === "" ? (
          "—"
        ) : (
          value
        )}
      </dd>
    </div>
  );
}
