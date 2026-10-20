import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { currentLocale } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { formatOffsetDateTime } from "./date-time-format";
import styles from "./date-time.module.css";

export function DateTime({
  value,
  options,
}: {
  value?: Timestamp | bigint | number;
  options?: Intl.DateTimeFormatOptions;
}) {
  useLocale();
  if (value === undefined) return <>—</>;
  const ms =
    typeof value === "object"
      ? Number(value.seconds) * 1000 + value.nanos / 1e6
      : Number(value);
  const date = new Date(ms);
  if (!Number.isFinite(date.getTime())) return <>—</>;
  return (
    <time className={styles.time} dateTime={date.toISOString()}>
      {options
        ? date.toLocaleString(currentLocale(), options)
        : formatOffsetDateTime(date)
            .split(/(\D+)/)
            .map((part, index) =>
              /\D/.test(part) ? (
                <span key={index} className={styles.separator}>
                  {part}
                </span>
              ) : (
                part
              ),
            )}
    </time>
  );
}
