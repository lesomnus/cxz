import { Icon } from "#src/shared/icons/icon.tsx";
import { useEffect, useState } from "react";
import { useLocalError } from "#src/shared/errors/error-context.tsx";
import {
  AnimatedCompactNumber,
  Button,
  CompactNumber,
  Popover,
  Section,
} from "@lesomnus/nox";
import type { ConversationStatsValues } from "#gen/cxz/session_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import usageStyles from "./usage-report.module.css";
import { UsageActivity } from "./usage-activity";
import { key } from "@lesomnus/payday/store";
import { accountReadPrefix, USAGE_TTL } from "#src/shared/api/read-cache.ts";
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

type UsageReportProps = {
  c: Connection;
  session?: string;
  project?: string;
  account?: string;
  accountId?: Uint8Array;
  className?: string;
  refreshVersion?: number;
  hideRefresh?: boolean;
  onLoadingChange?: (loading: boolean) => void;
};

export function UsageReport(props: UsageReportProps) {
  const { c, session = "", project = "", account = "", accountId } = props;
  const identity = accountId?.length ? key(accountId) : account;
  // A refresh keeps this view mounted; a different resource starts a clean view.
  return (
    <UsageReportView
      key={JSON.stringify([c.clientId, session, project, identity])}
      {...props}
    />
  );
}

function UsageReportView({
  c,
  session = "",
  project = "",
  account = "",
  accountId,
  className,
  refreshVersion = 0,
  hideRefresh = false,
  onLoadingChange,
}: UsageReportProps) {
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
  const identity = accountId?.length ? key(accountId) : account;
  const cachePrefix = identity
    ? `${accountReadPrefix(identity)}usage/`
    : `usage/${encodeURIComponent(JSON.stringify([session, project]))}/`;
  const scope = JSON.stringify([c.clientId, cachePrefix]);
  const request = {
    session: session ? ref(session) : undefined,
    project: !session && project ? ref(project) : undefined,
    account: identity
      ? accountId?.length
        ? { key: { case: "id" as const, value: accountId } }
        : { key: { case: "alias" as const, value: account } }
      : undefined,
  };
  const stats = useRemote(
    (signal) =>
      c.sessions.conversationStats(request, { signal, timeoutMs: 20000 }),
    [c, session, project, identity],
    {
      cache: { store: c.reads, key: `${cachePrefix}recent`, ttl: USAGE_TTL },
      refresh: refreshVersion,
    },
  );
  const history = useRemote(
    async (signal) => {
      if (!weeks || !stats.data) return;
      return c.sessions.conversationStats(
        { ...request, ...usageActivityRange(weeks, stats.data.toMs) },
        { signal, timeoutMs: 20000 },
      );
    },
    [c, session, project, identity, weeks, stats.data],
    {
      enabled: !!weeks && !!stats.data && !stats.loading,
      keepPreviousData: true,
      refresh: refreshVersion,
      cache:
        weeks && stats.data
          ? {
              store: c.reads,
              key: `${cachePrefix}history/${weeks}/${stats.data.toMs}`,
              ttl: USAGE_TTL,
            }
          : undefined,
    },
  );
  const readError = useLocalError(stats.error || history.error);
  useEffect(() => setNow(Date.now()), [refreshVersion]);
  useEffect(() => {
    onLoadingChange?.(stats.loading || history.loading);
  }, [onLoadingChange, stats.loading, history.loading]);
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
        {readError && (
          <Popover content={readError}>
            <span
              role="alert"
              aria-label={readError}
              className={usageStyles.error}
            >
              !
            </span>
          </Popover>
        )}
        {!hideRefresh && (
          <Button
            className={usageStyles.refresh}
            aria-label="Refresh"
            minLoadingDuration={500}
            loading={stats.loading || history.loading}
            onClick={async () => {
              c.reads.invalidatePrefix(cachePrefix);
              setNow(Date.now());
              await stats.reload();
            }}
          >
            <Icon name="refresh" width="16" height="16" aria-hidden="true" />
          </Button>
        )}
      </div>
      <section aria-label="Recent 30 days" className={usageStyles.period}>
        <strong className={usageStyles.periodTitle}>Recent 30 days</strong>
        <UsageMetrics
          values={stats.data?.total}
          unavailable={!stats.data?.total}
          animate={!stats.fromCache}
        />
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
        <UsageMetrics
          values={selectedValues}
          unavailable={!historyContainsDate && !stats.data}
          animate={!(historyContainsDate ? history.fromCache : stats.fromCache)}
        />
      </section>
      <UsageActivity
        stats={history.data}
        weeks={weeks ?? 1}
        toMs={stats.data?.toMs ?? BigInt(now)}
        loading={stats.loading || history.loading}
        selectedDate={date}
        onSelect={(day) => setSelection({ scope, day })}
        onWeeksChange={setWeeks}
      />
    </Section>
  );
}

function UsageMetrics({
  values,
  unavailable = false,
  animate = true,
}: {
  values?: ConversationStatsValues;
  unavailable?: boolean;
  animate?: boolean;
}) {
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
            {!unavailable && value === undefined ? (
              <span
                className={usageStyles.missing}
                role="img"
                aria-label="Not reported"
              >
                -
              </span>
            ) : (
              <span className={usageStyles.value}>
                {prefix && !unavailable && (
                  <span className={usageStyles.prefix} aria-hidden="true">
                    {prefix}
                  </span>
                )}
                <AnimatedCompactNumber
                  value={unavailable ? NaN : Number(value)}
                  label={unavailable ? "Unavailable" : `${prefix}${value}`}
                  muted={unavailable}
                  animate={animate}
                />
              </span>
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}
