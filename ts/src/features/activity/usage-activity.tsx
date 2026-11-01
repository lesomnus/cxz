import { CompactNumber, Popover } from "@lesomnus/nox";
import type { ConversationStatsReply } from "#gen/cxz/session_svc_pb";
import { usageActivityDays, type ActivityDay } from "./usage-activity-data";
import styles from "./usage-activity.module.css";

export function UsageActivity({ stats }: { stats: ConversationStatsReply }) {
  const days = usageActivityDays(stats);
  if (!days.length) return null;
  const offset = new Date(`${days[0].date}T00:00:00Z`).getUTCDay();
  const cells = [
    ...Array<null>(offset).fill(null),
    ...days,
    ...Array<null>((7 - ((offset + days.length) % 7)) % 7).fill(null),
  ];
  return (
    <section className={styles.activity} aria-label="Daily activity">
      <div className={styles.header}>
        <span>Daily activity</span>
        <span className={styles.period}>
          {days[0].date} – {days.at(-1)!.date} · UTC
        </span>
      </div>
      <div className={styles.calendar}>
        <div className={styles.weekdays} aria-hidden="true">
          {["", "Mon", "", "Wed", "", "Fri", ""].map((label, i) => (
            <span key={i}>{label}</span>
          ))}
        </div>
        <div className={styles.days}>
          {cells.map((day, i) => (
            <div className={styles.day} key={day?.date ?? i}>
              {day && (
                <Popover content={<DayDetails day={day} />}>
                  <span
                    className={styles.cell}
                    data-level={day.level}
                    role="img"
                    aria-label={`${day.date} (UTC): ${day.values?.finishedTurns ?? 0n} finished turns`}
                  />
                </Popover>
              )}
            </div>
          ))}
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

function DayDetails({ day }: { day: ActivityDay }) {
  const values = day.values;
  const metrics = [
    ["Finished turns", values?.finishedTurns ?? 0n],
    ["Input tokens", values?.inputTokens ?? 0n],
    ["Output tokens", values?.outputTokens ?? 0n],
    ["Cache read", values?.cacheReadTokens ?? 0n],
    ["Cache write", values?.cacheWriteTokens ?? 0n],
  ] as const;
  return (
    <span className={styles.details}>
      <strong>{day.date} · UTC</strong>
      {!values && <span className={styles.empty}>No recorded activity</span>}
      {metrics.map(([label, value]) => (
        <span className={styles.metric} key={label}>
          <span>{label}</span>
          <CompactNumber value={Number(value)} label={String(value)} />
        </span>
      ))}
      <span className={styles.metric}>
        <span>Reported cost</span>
        {values?.costReportedTurns ? (
          <span className={styles.cost}>
            <span aria-hidden="true">$</span>
            <CompactNumber
              value={values.reportedCostUsd}
              label={`$${values.reportedCostUsd}`}
            />
          </span>
        ) : (
          <span className={styles.missing} aria-label="Not reported">
            -
          </span>
        )}
      </span>
    </span>
  );
}
