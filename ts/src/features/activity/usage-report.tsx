import { Button } from "@lesomnus/cxz-ui";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import styles from "#src/shared/components/resource-surface.module.css";
export function UsageReport({
  c,
  session = "",
  project = "",
  account = "",
}: {
  c: Connection;
  session?: string;
  project?: string;
  account?: string;
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
  return (
    <section className={styles.section}>
      <div className={styles.row}>
        <strong>Conversation usage</strong>
        <Button onClick={stats.reload}>Refresh</Button>
      </div>
      <small className={styles.muted}>
        {account
          ? "Recorded conversation usage for this account's listed sessions over the last 30 days. Provider billing and remaining quota are not included."
          : "Retained conversation statistics for the last 30 days. These are reported token and cost metrics, not account billing or lifetime totals."}
      </small>
      {stats.error && <p role="alert">{stats.error}</p>}
      {stats.loading && <p className={styles.muted}>Loading usage…</p>}
      {total && (
        <>
          <dl className={styles.metrics}>
            {[
              ["Finished turns", total.finishedTurns],
              ["Input tokens", total.inputTokens],
              ["Output tokens", total.outputTokens],
              ["Cache read tokens", total.cacheReadTokens],
              ["Cache write tokens", total.cacheWriteTokens],
              ["Token-reported turns", total.tokenReportedTurns],
              [
                "Reported cost (USD)",
                total.costReportedTurns
                  ? total.reportedCostUsd.toFixed(4)
                  : "Not reported",
              ],
              ["Cost-reported turns", total.costReportedTurns],
            ].map(([label, value]) => (
              <div key={String(label)}>
                <dt>{String(label)}</dt>
                <dd>{String(value)}</dd>
              </div>
            ))}
          </dl>
          <small className={styles.muted}>
            {stats.data!.complete
              ? "Complete retained coverage"
              : "Partial coverage: some older records are unavailable"}
          </small>
          <details>
            <summary>Daily totals</summary>
            {stats.data?.days.map((day) => (
              <div className={styles.row} key={day.date}>
                <span>{day.date}</span>
                <span className={styles.status}>
                  {day.values?.finishedTurns.toString()} turns ·{" "}
                  {day.values?.outputTokens.toString()} output tokens
                </span>
              </div>
            ))}
          </details>
        </>
      )}
    </section>
  );
}
