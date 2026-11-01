import { CompactNumber, Section } from "@lesomnus/nox";
import { useEffect } from "react";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { readAccountQuota } from "./account-quota";
import styles from "./accounts.module.css";

export function AccountQuotaSquare({
  c,
  account,
  agent,
  sessions,
}: {
  c: Connection;
  account: string;
  agent: string;
  sessions: Session[];
}) {
  const identities = JSON.stringify(
    sessions.map((session) => session.runtimeId).sort(),
  );
  const quota = useRemote(
    (signal) => readAccountQuota(c, agent, sessions, signal),
    [c, account, agent, identities],
  );
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") quota.reload();
    };
    const timer = setInterval(refresh, 60000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      clearInterval(timer);
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [c, account, agent, identities]);
  const remaining = quota.data?.remaining;
  const percent = remaining === undefined ? undefined : Math.round(remaining);
  const level =
    remaining === undefined
      ? "unknown"
      : remaining < 10
        ? "critical"
        : remaining <= 30
          ? "warning"
          : "normal";
  return (
    <Section
      className={styles.quotaSquare}
      aria-label="Remaining agent usage"
      data-quota-level={level}
    >
      <span
        className={styles.quotaFill}
        style={{ height: `${remaining ?? 0}%` }}
        aria-hidden="true"
      />
      <span
        className={styles.quotaValue}
        role={percent === undefined ? "img" : "meter"}
        aria-label={
          percent === undefined
            ? quota.error
              ? "Usage unavailable"
              : "Not reported"
            : "Remaining agent usage"
        }
        aria-valuemin={percent === undefined ? undefined : 0}
        aria-valuemax={percent === undefined ? undefined : 100}
        aria-valuenow={percent}
        aria-valuetext={
          percent === undefined ? undefined : `${percent}% remaining`
        }
      >
        {percent === undefined ? (
          "—"
        ) : (
          <>
            <CompactNumber
              mode="integer"
              digits={percent === 100 ? 3 : 2}
              value={percent}
            />
            <span className={styles.quotaUnit}>%</span>
          </>
        )}
      </span>
    </Section>
  );
}
