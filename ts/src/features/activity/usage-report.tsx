import { Button, CompactNumber, Popover, Section } from "@lesomnus/nox";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import styles from "#src/shared/components/resource-surface.module.css";
import usageStyles from "./usage-report.module.css";
import { UsageActivity } from "./usage-activity";

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
  const stats = useRemote(
    (signal) =>
      c.sessions.conversationStats(
        {
          session: session ? ref(session) : undefined,
          project: !session && project ? ref(project) : undefined,
          account: account
            ? { key: { case: "alias", value: account } }
            : undefined,
        },
        { signal, timeoutMs: 20000 },
      ),
    [c, session, project, account],
  );
  const total = stats.data?.total;
  const metrics: UsageMetric[] = total
    ? [
        { label: "Input tokens", value: total.inputTokens },
        { label: "Output tokens", value: total.outputTokens },
        { label: "Cache read", value: total.cacheReadTokens },
        { label: "Cache write", value: total.cacheWriteTokens },
        { label: "Finished turns", value: total.finishedTurns },
        {
          label: "Token reports",
          value: total.tokenReportedTurns,
          description: "Turns with reported token usage.",
        },
        {
          label: "Cost reports",
          value: total.costReportedTurns,
          description: "Turns with a reported cost.",
        },
        {
          label: "Reported cost",
          value: total.costReportedTurns ? total.reportedCostUsd : undefined,
          prefix: "$",
        },
      ]
    : [];
  return (
    <Section
      className={`${usageStyles.report} ${className ?? ""}`}
      aria-label="Conversation usage"
    >
      <div className={`${styles.row} ${usageStyles.header}`}>
        <strong>Conversation usage</strong>
        <Button onClick={stats.reload}>Refresh</Button>
      </div>
      <small className={styles.muted}>
        {account
          ? "Recorded conversation usage for this account's sessions over the last 30 days. Provider billing and remaining quota are not included."
          : "Retained conversation statistics for the last 30 days. These are reported token and cost metrics, not account billing or lifetime totals."}
      </small>
      {stats.error && <p role="alert">{stats.error}</p>}
      {stats.loading && <p className={styles.muted}>Loading usage…</p>}
      {total && (
        <>
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
          <small className={styles.muted}>
            {stats.data!.complete
              ? "Complete retained coverage"
              : "Partial coverage: some older records are unavailable"}
          </small>
          <UsageActivity stats={stats.data!} />
        </>
      )}
    </Section>
  );
}
