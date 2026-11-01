import { useEffect, useState } from "react";
import { Button, CompactNumber, Popover, Section } from "@lesomnus/nox";
import type { ConversationStatsValues } from "#gen/cxz/session_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import usageStyles from "./usage-report.module.css";
import { UsageActivity } from "./usage-activity";
import {
  usageActivityRange,
  usageDateAge,
  type ActivityDay,
} from "./usage-activity-data";

type UsageMetric = {
  label: string;
  value: number | bigint | undefined;
  description?: string;
  prefix?: string;
};

export function UsageReport({
  c,
  session = "",
  project = "",
  account = "",
  className,
}: {
  c: Connection;
  session?: string;
  project?: string;
  account?: string;
  className?: string;
}) {
  const { locale } = useLocale();
  const [weeks, setWeeks] = useState<number>();
  const [selection, setSelection] = useState<{
    scope: string;
    day: ActivityDay;
  }>();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60000);
    return () => clearInterval(timer);
  }, []);
  const scope = JSON.stringify([c.clientId, session, project, account]);
  const request = {
    session: session ? ref(session) : undefined,
    project: !session && project ? ref(project) : undefined,
    account: account
      ? { key: { case: "alias" as const, value: account } }
      : undefined,
  };
  const stats = useRemote(
    (signal) =>
      c.sessions.conversationStats(request, { signal, timeoutMs: 20000 }),
    [c, session, project, account],
  );
  const history = useRemote(
    async (signal) => {
      if (!weeks || !stats.data) return;
      return c.sessions.conversationStats(
        { ...request, ...usageActivityRange(weeks, stats.data.toMs) },
        { signal, timeoutMs: 20000 },
      );
    },
    [c, session, project, account, weeks, stats.data],
  );
  const today = new Date(now).toISOString().slice(0, 10);
  const selected = selection?.scope === scope ? selection.day : undefined;
  const date = selected?.date ?? today;
  const age = usageDateAge(date, now, locale);
  const selectedMs = Date.parse(`${date}T00:00:00Z`);
  const historyContainsDate =
    history.data &&
    selectedMs >= Number(history.data.fromMs) &&
    selectedMs < Number(history.data.toMs);
  const selectedValues = historyContainsDate
    ? history.data?.days.find((day) => day.date === date)?.values
    : (stats.data?.days.find((day) => day.date === date)?.values ??
      selected?.values);
  return (
    <Section
      className={`${usageStyles.report} ${className ?? ""}`}
      aria-label="Conversation usage"
    >
      <div className={`${styles.row} ${usageStyles.header}`}>
        <strong>Conversation usage</strong>
        <Button
          onClick={() => {
            setNow(Date.now());
            stats.reload();
          }}
        >
          Refresh
        </Button>
      </div>
      {stats.error && <p role="alert">{stats.error}</p>}
      {stats.loading && <p className={styles.muted}>Loading usage…</p>}
      {stats.data?.total && (
        <>
          <section aria-label="Recent 30 days" className={usageStyles.period}>
            <strong className={usageStyles.periodTitle}>Recent 30 days</strong>
            <UsageMetrics values={stats.data.total} />
          </section>
          <section aria-label="Daily statistics" className={usageStyles.period}>
            <div className={usageStyles.dayHeader}>
              <strong className={usageStyles.periodTitle}>{date}</strong>
              <span className={usageStyles.date}>
                <time dateTime={date} aria-label={age.label}>
                  {age.parts.map((part, index) =>
                    part.type === "integer" ? (
                      <CompactNumber
                        key={index}
                        value={age.days}
                        label={part.value}
                        mode="integer"
                        digits={3}
                      />
                    ) : (
                      part.value
                    ),
                  )}
                </time>
                {" · UTC"}
              </span>
            </div>
            <UsageMetrics values={selectedValues} />
          </section>
          {history.error && <p role="alert">{history.error}</p>}
          <UsageActivity
            stats={history.data}
            loading={history.loading}
            selectedDate={date}
            onSelect={(day) => setSelection({ scope, day })}
            onWeeksChange={setWeeks}
          />
          {(!stats.data.complete ||
            (history.data && !history.data.complete)) && (
            <small className={styles.muted}>
              Partial coverage: some older records are unavailable
            </small>
          )}
        </>
      )}
    </Section>
  );
}

function UsageMetrics({ values }: { values?: ConversationStatsValues }) {
  const metrics: UsageMetric[] = [
    { label: "Input tokens", value: values?.inputTokens ?? 0n },
    { label: "Output tokens", value: values?.outputTokens ?? 0n },
    { label: "Cache read", value: values?.cacheReadTokens ?? 0n },
    { label: "Cache write", value: values?.cacheWriteTokens ?? 0n },
    { label: "Finished turns", value: values?.finishedTurns ?? 0n },
    {
      label: "Token reports",
      value: values?.tokenReportedTurns ?? 0n,
      description: "Turns with reported token usage.",
    },
    {
      label: "Cost reports",
      value: values?.costReportedTurns ?? 0n,
      description: "Turns with a reported cost.",
    },
    {
      label: "Reported cost",
      value: values?.costReportedTurns ? values.reportedCostUsd : undefined,
      prefix: "$",
    },
  ];
  return (
    <dl className={`${styles.metrics} ${usageStyles.metrics}`}>
      {metrics.map(({ label, value, description, prefix = "" }) => (
        <div key={label}>
          <dt>
            {description ? (
              <Popover content={description}>
                <span>{label}</span>
              </Popover>
            ) : (
              label
            )}
          </dt>
          <dd>
            {value === undefined ? (
              <span
                className={usageStyles.missing}
                role="img"
                aria-label="Not reported"
              >
                -
              </span>
            ) : (
              <span className={usageStyles.value}>
                {prefix && (
                  <span className={usageStyles.prefix} aria-hidden="true">
                    {prefix}
                  </span>
                )}
                <CompactNumber
                  value={Number(value)}
                  label={`${prefix}${value}`}
                />
              </span>
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}
