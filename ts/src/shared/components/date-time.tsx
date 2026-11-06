import { useEffect, useState } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  dateTimeDate,
  formatLocalDateTime,
  formatOffsetDateTime,
  formatRelativeDateTime,
  type DateTimeValue,
} from "./date-time-format";
import styles from "./date-time.module.css";

export function DateTime({
  value,
  title,
  showOffset = true,
}: {
  value?: DateTimeValue;
  title?: string;
  showOffset?: boolean;
}) {
  useLocale();
  const date = dateTimeDate(value);
  if (!date) return <>—</>;
  return (
    <time className={styles.time} dateTime={date.toISOString()} title={title}>
      {(showOffset ? formatOffsetDateTime(date) : formatLocalDateTime(date))
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

// A separate label detail keeps the absolute timestamp available in the value.
export function RelativeTime({ value }: { value?: DateTimeValue }) {
  const { locale } = useLocale();
  const [now, setNow] = useState(Date.now);
  const date = dateTimeDate(value);
  const valid = date !== undefined;
  useEffect(() => {
    if (!valid) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, [valid]);
  if (!date) return null;
  return (
    <time className={styles.time} dateTime={date.toISOString()}>
      {formatRelativeDateTime(date, locale, now)}
    </time>
  );
}
