import { ActivityHeatmap } from "@lesomnus/nox";
import { useLayoutEffect, useMemo, useRef } from "react";
import type { ConversationStatsReply } from "#gen/cxz/session_svc_pb";
import {
  usageActivityDays,
  usageActivityRange,
  type ActivityDay,
} from "./usage-activity-data";
export function UsageActivity({
  stats,
  loading,
  weeks,
  toMs,
  selectedDate,
  onSelect,
  onWeeksChange,
  animate = false,
  today = new Date().toISOString().slice(0, 10),
}: {
  stats?: ConversationStatsReply;
  loading: boolean;
  weeks: number;
  toMs: bigint;
  selectedDate: string;
  onSelect: (day: ActivityDay) => void;
  onWeeksChange: (weeks: number) => void;
  animate?: boolean;
  today?: string;
}) {
  const element = useRef<HTMLElement>(null);
  const previous = useRef(
    new Map<string, { signature: string; color: string }>(),
  );
  const days = useMemo(
    () =>
      usageActivityDays(
        stats ?? { ...usageActivityRange(weeks, toMs), days: [] },
      ).map((day) => ({
        ...day,
        label: `${day.date} (UTC): ${day.values?.finishedTurns ?? 0n} finished turns`,
      })),
    [stats, weeks, toMs],
  );
  useLayoutEffect(() => {
    const root = element.current;
    if (!root || loading) return;
    const cells = [
      ...root.querySelectorAll<HTMLButtonElement>(
        "[data-date] > button[data-level]",
      ),
    ];
    const byDate = new Map(days.map((day) => [day.date, day]));
    // Keep existing colors through refreshes; cached or unchanged days stay still.
    const before = previous.current;
    previous.current = new Map();
    const motion = matchMedia("(prefers-reduced-motion: reduce)");
    const origin = root
      .querySelector<HTMLElement>(`[data-date="${today}"]`)
      ?.getBoundingClientRect();
    const animations: Animation[] = [];
    for (const cell of cells) {
      const date = cell.parentElement!.dataset.date!;
      const day = byDate.get(date)!;
      const signature = `${day.level}:${day.values?.finishedTurns ?? 0n}`;
      const style = getComputedStyle(cell);
      const color = style.backgroundColor;
      previous.current.set(date, { signature, color });
      if (
        !animate ||
        motion.matches ||
        !origin ||
        day.level <= 0 ||
        before.get(date)?.signature === signature
      )
        continue;
      // Calendar rows wrap by week, so use screen distance rather than date age.
      const bounds = cell.getBoundingClientRect();
      const distance = Math.hypot(
        bounds.x + bounds.width / 2 - origin.x - origin.width / 2,
        bounds.y + bounds.height / 2 - origin.y - origin.height / 2,
      );
      animations.push(
        cell.animate(
          [
            {
              backgroundColor:
                before.get(date)?.color ??
                style.getPropertyValue("--activity-month-color").trim(),
            },
            { backgroundColor: color },
          ],
          {
            duration: 320,
            delay: Math.min(600, distance * 1.2),
            easing: "ease-out",
            fill: "backwards",
          },
        ),
      );
    }
    const cancel = () => animations.forEach((animation) => animation.cancel());
    const changed = () => {
      if (motion.matches) cancel();
    };
    motion.addEventListener("change", changed);
    return () => {
      motion.removeEventListener("change", changed);
      cancel();
    };
  }, [days, loading, animate, today]);
  return (
    <ActivityHeatmap
      ref={element}
      days={days}
      loading={loading}
      placeholder={!stats}
      selectedDate={selectedDate}
      onSelect={onSelect}
      onWeeksChange={onWeeksChange}
      moreLabel="More turns"
      legendLabel="Color intensity: finished turns per day"
    />
  );
}
