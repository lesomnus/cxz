import { DateTime } from "#src/shared/components/date-time.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
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
      <DateTime value={timeMs} />
    </span>
  );
}
