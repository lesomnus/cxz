const MINUTE = 60_000;
const DAY = 24 * 60 * MINUTE;
const absolute = new Intl.DateTimeFormat("ko-KR", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

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
  if (age < MINUTE) return "1분 이내";
  if (age < 60 * MINUTE) return `${Math.floor(age / MINUTE)}분 전`;
  if (age < DAY) return `${Math.floor(age / (60 * MINUTE))}시간 전`;
  // Compare local calendar dates without letting DST change the day count.
  const day = (d: Date) =>
    Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / DAY;
  const days = day(now) - day(date);
  if (days === 1) return "어제";
  const monday = day(now) - ((now.getDay() + 6) % 7);
  if (days >= 4 && day(date) < monday) return "지난주";
  return `${days}일 전`;
}

export function absoluteMessageTime(date: Date) {
  return absolute.format(date);
}
