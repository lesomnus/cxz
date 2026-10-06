import { useEffect, useState } from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import {
  absoluteMessageTime,
  messageDate,
  relativeMessageTime,
} from "./message-time";

function InputTime({ timeMs }: { timeMs: bigint }) {
  const [now, setNow] = useState(() => new Date());
  const date = messageDate(timeMs);
  const recent = !!date && now.getTime() - date.getTime() < 7 * 86_400_000;
  useEffect(() => {
    if (!recent) return;
    const timer = setInterval(() => setNow(new Date()), 30_000);
    return () => clearInterval(timer);
  }, [recent, timeMs]);
  if (!date) return null;
  const relative = relativeMessageTime(date, now);
  return (
    <div className="input-time">
      <time dateTime={date.toISOString()} title={date.toLocaleString("ko-KR")}>
        {absoluteMessageTime(date)}
      </time>
      {relative && <span className="input-relative-time">{relative}</span>}
    </div>
  );
}

// Shared by the transcript and the fixed preview of the preceding input.
export function InputMessage({ event }: { event: SessionEvent }) {
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
}
