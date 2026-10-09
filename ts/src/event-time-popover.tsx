import { currentLocale } from "./i18n";
import { useLocale } from "./i18n-react";
import { messageDate } from "./message-time";

// Display the journal's recorded timestamp, never the time of a rerender.
export function EventTimePopover({
  timeMs,
  id,
}: {
  timeMs: bigint;
  id: string;
}) {
  useLocale();
  const date = messageDate(timeMs);
  if (!date) return null;
  return (
    <span
      id={id}
      className="meta-tooltip event-time-tooltip"
      role="tooltip"
      aria-hidden="true"
    >
      <time dateTime={date.toISOString()}>
        {date.toLocaleString(currentLocale(), {
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          hour12: false,
        })}
      </time>
    </span>
  );
}
