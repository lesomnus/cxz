import { useLayoutEffect, useRef } from "react";
import type { ConversationStatsReply } from "#gen/cxz/session_svc_pb";
import {
  usageActivityDays,
  usageActivityCells,
  usageActivityWeeks,
  type ActivityDay,
} from "./usage-activity-data";
import styles from "./usage-activity.module.css";

export function UsageActivity({
  stats,
  loading,
  selectedDate,
  onSelect,
  onWeeksChange,
}: {
  stats?: ConversationStatsReply;
  loading: boolean;
  selectedDate: string;
  onSelect: (day: ActivityDay) => void;
  onWeeksChange: (weeks: number) => void;
}) {
  const grid = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const element = grid.current;
    if (!element) return;
    const measure = () => {
      const style = getComputedStyle(element);
      const size = parseFloat(style.getPropertyValue("--activity-cell-size"));
      const gap = parseFloat(style.rowGap);
      const width = element.getBoundingClientRect().width;
      const weeks = usageActivityWeeks(width, size, gap);
      const columnGap = weeks > 1 ? (width - weeks * size) / (weeks - 1) : gap;
      element.style.setProperty("--activity-column-gap", `${columnGap}px`);
      onWeeksChange(weeks);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [onWeeksChange]);
  const days = stats ? usageActivityDays(stats) : [];
  const cells = usageActivityCells(days);
  return (
    <section
      className={styles.activity}
      aria-label="Daily activity"
      aria-busy={loading}
    >
      <div className={styles.header}>
        <span>Daily activity</span>
        {days.length > 0 && (
          <span className={styles.period}>
            {days[0].date} – {days.at(-1)!.date} · UTC
          </span>
        )}
      </div>
      <div className={styles.calendar}>
        <div className={styles.weekdays} aria-hidden="true">
          {["", "Mon", "", "Wed", "", "Fri", ""].map((label, i) => (
            <span key={i}>{label}</span>
          ))}
        </div>
        <div className={styles.days} ref={grid}>
          {cells.map(({ date, day, future, top, bottom, left, right }) =>
            day || future ? (
              <div
                key={date}
                className={styles.day}
                data-date={date}
                data-month={date.slice(0, 7)}
                data-top={top || undefined}
                data-bottom={bottom || undefined}
                data-left={left || undefined}
                data-right={right || undefined}
              >
                {day ? (
                  <button
                    type="button"
                    className={styles.cell}
                    data-level={day.level}
                    aria-pressed={selectedDate === date}
                    aria-label={`${date} (UTC): ${day.values?.finishedTurns ?? 0n} finished turns`}
                    onClick={() => onSelect(day)}
                  />
                ) : (
                  <span
                    className={styles.cell}
                    data-future="true"
                    aria-hidden="true"
                  />
                )}
              </div>
            ) : (
              <span key={date} aria-hidden="true" />
            ),
          )}
        </div>
      </div>
      <div
        className={styles.legend}
        aria-label="Color intensity: finished turns per day"
      >
        <span>Less</span>
        {[0, 1, 2, 3, 4].map((level) => (
          <span
            key={level}
            className={styles.cell}
            data-level={level}
            aria-hidden="true"
          />
        ))}
        <span>More turns</span>
      </div>
    </section>
  );
}
