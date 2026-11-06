import type { Timestamp } from "@bufbuild/protobuf/wkt";

export type DateTimeValue = Timestamp | bigint | number;

export function dateTimeDate(value?: DateTimeValue) {
  if (value === undefined) return undefined;
  const ms =
    typeof value === "object"
      ? Number(value.seconds) * 1000 + value.nanos / 1e6
      : Number(value);
  const date = new Date(ms);
  return Number.isFinite(date.getTime()) ? date : undefined;
}

export function formatRelativeDateTime(
  date: Date,
  locale: string,
  now = Date.now(),
) {
  const elapsed = date.getTime() - now;
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const units = [
    [365 * 86400000, "year"],
    [30 * 86400000, "month"],
    [7 * 86400000, "week"],
    [86400000, "day"],
    [3600000, "hour"],
    [60000, "minute"],
  ] as const;
  for (const [duration, unit] of units) {
    if (Math.abs(elapsed) >= duration)
      return format.format(Math.trunc(elapsed / duration), unit);
  }
  return format.format(0, "second");
}

export function formatLocalDateTime(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return (
    `${String(date.getFullYear()).padStart(4, "0")}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
}

export function formatOffsetDateTime(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  const offset = -date.getTimezoneOffset();
  const magnitude = Math.abs(offset);
  return (
    formatLocalDateTime(date) +
    `${offset < 0 ? "-" : "+"}${pad(Math.floor(magnitude / 60))}:${pad(magnitude % 60)}`
  );
}
