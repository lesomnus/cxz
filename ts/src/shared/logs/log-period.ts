import type { LogQuery } from "./log-entry";

/** Calendar dates are local days; the selected end day is included in full. */
export function logPeriodBounds(
  start: string,
  end: string,
): Pick<LogQuery, "since" | "until"> {
  const nextDay = end ? new Date(`${end}T00:00:00`) : undefined;
  nextDay?.setDate(nextDay.getDate() + 1);
  return {
    since: start ? new Date(`${start}T00:00:00`).getTime() : undefined,
    until: nextDay?.getTime(),
  };
}
