import { t, currentLocale } from "./i18n";
const MINUTE = 60_000;
const DAY = 24 * 60 * MINUTE;
const absoluteOptions: Intl.DateTimeFormatOptions = {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
};

export function messageDate(timeMs: bigint) {
  const value = Number(timeMs);
  const date = new Date(value);
  return value > 0 &&
    Number.isSafeInteger(value) &&
    !Number.isNaN(date.getTime())
    ? date
    : undefined;
}

export function relativeMessageTime(date: Date, now = new Date()) {
  const age = now.getTime() - date.getTime();
  if (age < 0 || age >= 7 * DAY) return "";
  if (age < MINUTE) return t("Less than a minute ago");
  if (age < 60 * MINUTE)
    return t(
      Math.floor(age / MINUTE) === 1
        ? "{count} minute ago"
        : "{count} minutes ago",
      { count: Math.floor(age / MINUTE) },
    );
  if (age < DAY)
    return t(
      Math.floor(age / (60 * MINUTE)) === 1
        ? "{count} hour ago"
        : "{count} hours ago",
      { count: Math.floor(age / (60 * MINUTE)) },
    );
  // Compare local calendar dates without letting DST change the day count.
  const day = (d: Date) =>
    Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / DAY;
  const days = day(now) - day(date);
  if (days === 1) return t("Yesterday");
  const monday = day(now) - ((now.getDay() + 6) % 7);
  if (days >= 4 && day(date) < monday) return t("Last week");
  return t("{count} days ago", { count: days });
}

export function absoluteMessageTime(date: Date) {
  return new Intl.DateTimeFormat(currentLocale(), absoluteOptions).format(date);
}
