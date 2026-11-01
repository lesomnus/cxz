import type {
  ConversationStatsDay,
  ConversationStatsReply,
} from "#gen/cxz/session_svc_pb";

const dayMs = 86400000;

export function usageActivityWeeks(width: number, size: number, gap: number) {
  return Math.max(1, Math.min(52, Math.floor((width + gap) / (size + gap))));
}

export function usageActivityRange(weeks: number, toMs: bigint) {
  const last = new Date(Number(toMs) - 1);
  const midnight = Date.UTC(
    last.getUTCFullYear(),
    last.getUTCMonth(),
    last.getUTCDate(),
  );
  const fromMs = BigInt(
    midnight - (last.getUTCDay() + (weeks - 1) * 7) * dayMs,
  );
  return { fromMs, toMs };
}

export function usageDateAge(date: string, now: number, locale: string) {
  const today = Math.floor(now / dayMs) * dayMs;
  const days = Math.round((Date.parse(`${date}T00:00:00Z`) - today) / dayMs);
  return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(
    days,
    "day",
  );
}

export type ActivityDay = {
  date: string;
  values: ConversationStatsDay["values"];
  level: number;
};

// Calendar neighbours are one day vertically and seven days horizontally.
// Extend only the last week, keeping future days outside the recorded data.
export function usageActivityCells(days: ActivityDay[]) {
  if (!days.length) return [];
  const offset = new Date(`${days[0].date}T00:00:00Z`).getUTCDay();
  const first = Date.parse(`${days[0].date}T00:00:00Z`) - offset * dayMs;
  const count = Math.ceil((offset + days.length) / 7) * 7;
  const cells = Array.from({ length: count }, (_, i) => ({
    date: new Date(first + i * dayMs).toISOString().slice(0, 10),
    day: days[i - offset] as ActivityDay | undefined,
    future: i >= offset + days.length,
  }));
  return cells.map((cell, i) => {
    const sameMonth = (index: number) => {
      const neighbour = cells[index];
      return (
        !!neighbour &&
        !!(neighbour.day || neighbour.future) &&
        cell.date.slice(0, 7) === neighbour.date.slice(0, 7)
      );
    };
    return {
      ...cell,
      top: i % 7 > 0 && sameMonth(i - 1),
      bottom: i % 7 < 6 && sameMonth(i + 1),
      left: sameMonth(i - 7),
      right: sameMonth(i + 7),
    };
  });
}

// Stats use UTC dates and a half-open time range. Missing dates have no
// recorded activity; padding outside the requested range is not a zero day.
export function usageActivityDays({
  fromMs,
  toMs,
  days,
}: Pick<ConversationStatsReply, "fromMs" | "toMs" | "days">): ActivityDay[] {
  const from = Number(fromMs);
  const to = Number(toMs);
  if (to <= from) return [];
  const recorded = new Map(days.map((day) => [day.date, day.values]));
  const result: ActivityDay[] = [];
  for (let time = Math.floor(from / dayMs) * dayMs; time < to; time += dayMs) {
    const date = new Date(time).toISOString().slice(0, 10);
    result.push({ date, values: recorded.get(date), level: 0 });
  }
  const peak = result.reduce((max, day) => {
    const turns = day.values?.finishedTurns ?? 0n;
    return turns > max ? turns : max;
  }, 0n);
  for (const day of result) {
    const turns = day.values?.finishedTurns ?? 0n;
    // Integer buckets preserve small nonzero counts beside very busy days.
    if (turns > 0n) day.level = Number((turns * 4n - 1n) / peak + 1n);
  }
  return result;
}
