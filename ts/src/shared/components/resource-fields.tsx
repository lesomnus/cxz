import { useEffect, useState, type ReactNode } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { IdentifierValue } from "./identifier-value";
import {
  FieldLabel,
  PathValue,
  DescriptionListRoot,
  DescriptionListRow,
  DescriptionListTerm,
  DescriptionListDetails,
  DescriptionListAnnotation,
} from "@lesomnus/nox";

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
  variant = "default",
  density = "default",
}: {
  rows: readonly ResourceFieldRow[];
  /** Equal-width count, or relative column weights such as [1, 2]. */
  columns?: 1 | 2 | 3 | readonly number[];
  /** Input/readonly values share Form spacing, surface insets and section gaps. */
  variant?: "default" | "controls";
  /** Compact controls retain the tighter Account editable-value alignment. */
  density?: "default" | "compact";
}) {
  return (
    <DescriptionListRoot columns={columns} variant={variant} density={density}>
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
    </DescriptionListRoot>
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
    <DescriptionListRow fullWidth={fullWidth}>
      <DescriptionListTerm>
        <FieldLabel label={label} description={description} />
        {labelDetail != null && (
          <DescriptionListAnnotation>{labelDetail}</DescriptionListAnnotation>
        )}
        {(path || identifier) && (
          <DescriptionListAnnotation error={feedback?.failed} role="status">
            {feedback?.message}
          </DescriptionListAnnotation>
        )}
      </DescriptionListTerm>
      <DescriptionListDetails>
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
      </DescriptionListDetails>
    </DescriptionListRow>
  );
}
