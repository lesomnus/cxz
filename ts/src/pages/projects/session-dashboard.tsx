import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import { Link } from "@tanstack/react-router";
import type { Session } from "#gen/cxz/session_pb";
import { ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { t, currentLocale } from "#src/shared/i18n/i18n.ts";
import { eventDiagnostic } from "#src/features/session/model/event-diagnostic.ts";
import { questions } from "#src/features/session/model/journal.ts";
import {
  formatTokens,
  formatReset,
} from "#src/features/session/model/session-info.ts";
import {
  recentSession,
  RECENT_SESSION_ROWS,
  type DashboardConnection,
} from "./project-dashboard";
import styles from "./projects-page.module.css";

export function date(value: Session["dateCreated"]) {
  return value
    ? new Date(Number(value.seconds) * 1000 + value.nanos / 1e6).toLocaleString(
        currentLocale(),
      )
    : "—";
}
export function Fields({
  rows,
}: {
  rows: [string, string | number | undefined][];
}) {
  return (
    <dl className={styles.details}>
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value === undefined || value === "" ? "—" : value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function SessionDashboard({
  c,
  session: s,
  id,
}: {
  c: DashboardConnection;
  session: Session;
  id: string;
}) {
  // Coalesce live inventory updates. Only the selected session reads a small
  // recent page; continuous tool events never cause one request per event.
  const latest = useRef(s.status?.lastSeq);
  latest.current = s.status?.lastSeq;
  const [revision, setRevision] = useState(s.status?.lastSeq);
  const reload = useRef<() => void>(() => {});
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const history = useRemote(
    (signal) =>
      c.sessions.transcript(
        { ref: ref(s.runtimeId), limit: RECENT_SESSION_ROWS },
        { signal, timeoutMs: 15000 },
      ),
    [c, s.runtimeId, s.status?.runId],
  );
  useEffect(() => () => clearTimeout(timer.current), []);
  useEffect(() => {
    if (history.loading) {
      clearTimeout(timer.current);
      timer.current = undefined;
      return;
    }
    if (revision === latest.current || timer.current) return;
    timer.current = setTimeout(() => {
      timer.current = undefined;
      setRevision(latest.current);
      reload.current();
    }, 3000);
  }, [s.status?.lastSeq, revision, history.loading]);
  reload.current = history.reload;
  const snapshot = useMemo(
    () =>
      recentSession(
        s,
        history.data?.events ?? [],
        history.data?.metadata ?? [],
      ),
    [s, history.data],
  );
  const account = useRemote(
    (signal) =>
      s.account?.name || s.account?.alias || !s.account?.id.length
        ? Promise.resolve(s.account)
        : c.accounts.get(
            { ref: { key: { case: "id", value: s.account.id } } },
            { signal, timeoutMs: 10000 },
          ),
    [c, s.account ? key(s.account.id) : "", s.account?.name, s.account?.alias],
  );
  const info = snapshot.info;
  const pending = s.status?.pending ?? [];
  return (
    <section
      id={id}
      aria-label={t("Session details")}
      className={styles.section}
    >
      <div className={styles.sectionHeading}>
        <h2>{t("Session details")}</h2>
        <Link to="/sessions/$sessionId" params={{ sessionId: s.runtimeId }}>
          {t("Open conversation")}
        </Link>
      </div>
      <Fields
        rows={[
          [t("State"), s.status?.state],
          [t("Agent"), s.agent],
          [t("Account"), account.data?.name || account.data?.alias],
          [t("Current model"), info.model],
          [t("Effort"), info.effort],
          [t("Configured model"), s.model],
          [
            t("Remaining usage"),
            info.remaining === undefined
              ? undefined
              : `${Math.round(info.remaining)}%`,
          ],
          [
            t("Usage reset"),
            info.reset === undefined ? undefined : formatReset(info.reset),
          ],
          [
            t("Context"),
            info.contextUsed === undefined || info.contextWindow === undefined
              ? undefined
              : `${formatTokens(info.contextUsed)} / ${formatTokens(info.contextWindow)}`,
          ],
          [t("Created"), date(s.dateCreated)],
          [t("Updated"), date(s.dateUpdated)],
        ]}
      />
      {history.loading && (
        <p className="muted" role="status">
          {t("Loading recent activity…")}
        </p>
      )}
      {history.error && (
        <div>
          <p role="alert">{history.error}</p>
          <Button type="button" onClick={history.reload}>
            {t("Retry")}
          </Button>
        </div>
      )}
      {account.error && <p role="alert">{account.error}</p>}
      <h3 className={styles.subheading}>{t("Waiting for input")}</h3>
      {pending.length ? (
        <ul className={styles.activity}>
          {pending.map((e) => (
            <li key={e.seq.toString()}>
              {questions(s.agent, e)
                .map((q) => q.text)
                .join(" · ") ||
                e.text ||
                t("Approval")}
            </li>
          ))}
        </ul>
      ) : (
        <p className={styles.empty}>{t("No pending requests")}</p>
      )}
      {s.status?.queued && (
        <>
          <h3 className={styles.subheading}>{t("Queued message")}</h3>
          <p className={styles.preview}>{s.status.queued}</p>
        </>
      )}
      <h3 className={styles.subheading}>{t("Recent activity")}</h3>
      <ul className={styles.activity}>
        {snapshot.activity.map((e) => (
          <li key={e.seq.toString()}>
            <span className={styles.activityMeta}>
              {e.kind === "input" ? t("You") : s.agent} ·{" "}
              {new Date(Number(e.timeMs)).toLocaleString(currentLocale())}
            </span>
            <p className={styles.preview}>{e.text}</p>
          </li>
        ))}
      </ul>
      {!history.loading && !history.error && !snapshot.activity.length && (
        <p className={styles.empty}>{t("No recent activity")}</p>
      )}
      <h3 className={styles.subheading}>{t("Recent errors")}</h3>
      <ul className={`${styles.activity} ${styles.errors}`}>
        {snapshot.errors.map((e) => (
          <li key={e.seq.toString()}>
            <span className={styles.activityMeta}>
              {new Date(Number(e.timeMs)).toLocaleString(currentLocale())}
            </span>
            <p className={styles.preview}>{eventDiagnostic(e)?.message}</p>
          </li>
        ))}
      </ul>
      {!history.loading && !history.error && !snapshot.errors.length && (
        <p className={styles.empty}>{t("No errors in recent records")}</p>
      )}
      <p className={styles.empty}>
        {t(
          "Activity and errors show only the latest records. Unreported metrics are shown as —.",
        )}
      </p>
      <details className={styles.diagnostics}>
        <summary>{t("Diagnostics")}</summary>
        <Fields
          rows={[
            [t("Runtime ID"), s.runtimeId],
            [t("Run ID"), s.status?.runId],
            [t("Provider session ID"), s.status?.vendorId],
            [t("Permission mode"), s.status?.permissionMode],
            [t("Last event"), s.status?.lastSeq.toString()],
          ]}
        />
      </details>
    </section>
  );
}
