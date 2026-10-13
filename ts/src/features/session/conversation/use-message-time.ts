import { useEffect, useState } from "react";
import { messageDate, relativeMessageTime } from "./message-time";

// Shared by input and assistant timestamps. Stop updating after the relative
// label expires, and dispose the timer with its virtualized message row.
export function useMessageTime(timeMs: bigint) {
  const [now, setNow] = useState(() => new Date());
  const date = messageDate(timeMs);
  const recent = !!date && now.getTime() - date.getTime() < 7 * 86_400_000;
  useEffect(() => {
    if (!recent) return;
    const timer = setInterval(() => setNow(new Date()), 30_000);
    return () => clearInterval(timer);
  }, [recent, timeMs]);
  return { date, relative: date ? relativeMessageTime(date, now) : "" };
}
