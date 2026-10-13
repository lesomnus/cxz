import { currentLocale } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { absoluteMessageTime } from "./message-time";
import { useMessageTime } from "./use-message-time";
import { memo } from "react";

function InputTime({ timeMs }: { timeMs: bigint }) {
  useLocale();
  const { date, relative } = useMessageTime(timeMs);
  if (!date) return null;
  return (
    <div className="input-time">
      <time
        dateTime={date.toISOString()}
        title={date.toLocaleString(currentLocale())}
      >
        {absoluteMessageTime(date)}
      </time>
      {relative && <span className="input-relative-time">- {relative}</span>}
    </div>
  );
}

// Shared by the transcript and the fixed preview of the preceding input.
export const InputMessage = memo(function InputMessage({
  event,
}: {
  event: SessionEvent;
}) {
  return (
    <div className="input-box">
      <InputTime timeMs={event.timeMs} />
      <div className="input-content">
        <span className="input-prefix" aria-hidden="true">
          &gt;
        </span>
        <p className="message-body">{event.text}</p>
      </div>
    </div>
  );
});
