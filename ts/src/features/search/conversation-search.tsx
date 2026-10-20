import { DateTime } from "#src/shared/components/date-time.tsx";
import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import type {
  SessionSearchVisit,
  SessionSearchSummary,
} from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import styles from "#src/shared/components/resource-surface.module.css";

export function ConversationSearch({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState("substring");
  const [project, setProject] = useState("");
  const [tools, setTools] = useState(false);
  const [ignoreCase, setIgnoreCase] = useState(true);
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [visits, setVisits] = useState<SessionSearchVisit[]>([]);
  const [summary, setSummary] = useState<SessionSearchSummary>();
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const active = useRef<AbortController | undefined>(undefined);
  const submitted = useRef({
    query: "",
    match,
    includeTools: tools,
    ignoreCase,
    projects: [] as string[],
    since: undefined as { seconds: bigint; nanos: number } | undefined,
    until: undefined as { seconds: bigint; nanos: number } | undefined,
  });
  useEffect(() => () => active.current?.abort(), []);
  function invalidate() {
    active.current?.abort();
    setLoading(false);
    setSummary(undefined);
  }
  async function search(more = false) {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    if (!more) {
      setVisits([]);
      setSummary(undefined);
      const stamp = (date: string) =>
        date
          ? {
              seconds: BigInt(
                Math.floor(Date.parse(date + "T00:00:00Z") / 1000),
              ),
              nanos: 0,
            }
          : undefined;
      submitted.current = {
        query,
        match,
        includeTools: tools,
        ignoreCase,
        projects: project ? [project] : [],
        since: stamp(since),
        until: stamp(until),
      };
    }
    setLoading(true);
    setError("");
    setProgress("");
    try {
      for await (const reply of c.sessions.search(
        {
          ...submitted.current,
          cursor: more ? summary?.nextCursor : undefined,
          limit: 100,
          snippet: 240,
          clientId: c.clientId,
        },
        { signal: controller.signal, timeoutMs: 60000 },
      )) {
        if (controller.signal.aborted) return;
        if (reply.visit)
          setVisits((old) => {
            const found = old.find(
              (v) => v.sessionId === reply.visit!.sessionId,
            );
            return found
              ? old.map((v) =>
                  v === found
                    ? {
                        ...v,
                        hits: [
                          ...new Map(
                            [...v.hits, ...reply.visit!.hits].map((h) => [
                              h.seq,
                              h,
                            ]),
                          ).values(),
                        ],
                      }
                    : v,
                )
              : [...old, reply.visit!];
          });
        if (reply.progress)
          setProgress(
            `${reply.progress.projectName}: ${reply.progress.state} ${reply.progress.message}`,
          );
        if (reply.summary) setSummary(reply.summary);
      }
    } catch (error) {
      if (!controller.signal.aborted) setError(String(error));
    } finally {
      if (active.current === controller) setLoading(false);
    }
  }
  return (
    <div className={styles.content}>
      <form
        className={styles.section}
        onSubmit={(e) => {
          e.preventDefault();
          void search();
        }}
      >
        <label className={styles.field}>
          Search conversations
          <input
            value={query}
            onChange={(e) => {
              invalidate();
              setQuery(e.target.value);
            }}
          />
        </label>
        <div className={styles.row}>
          <ValueMenu
            label="Match"
            value={match}
            options={["substring", "regex", "fuzzy"].map((value) => ({
              value,
              label: value,
            }))}
            choose={(v) => {
              invalidate();
              setMatch(v);
            }}
          />
          <ValueMenu
            label="Project"
            value={project}
            options={[
              { value: "", label: "All projects" },
              ...projects.map((p) => ({
                value: p.runtimeId,
                label: p.name || p.alias,
              })),
            ]}
            choose={(v) => {
              invalidate();
              setProject(v);
            }}
          />
        </div>
        <div className={styles.row}>
          <label>
            <input
              type="checkbox"
              checked={ignoreCase}
              onChange={(e) => {
                invalidate();
                setIgnoreCase(e.target.checked);
              }}
            />{" "}
            Ignore case
          </label>
          <label>
            <input
              type="checkbox"
              checked={tools}
              onChange={(e) => {
                invalidate();
                setTools(e.target.checked);
              }}
            />{" "}
            Include tools
          </label>
        </div>
        <div className={styles.row}>
          <label className={styles.field}>
            From (UTC)
            <input
              type="date"
              value={since}
              onChange={(e) => {
                invalidate();
                setSince(e.target.value);
              }}
            />
          </label>
          <label className={styles.field}>
            Until (UTC, exclusive)
            <input
              type="date"
              value={until}
              onChange={(e) => {
                invalidate();
                setUntil(e.target.value);
              }}
            />
          </label>
        </div>
        <div className={styles.row}>
          <Button type="submit" disabled={loading || !query.trim()}>
            Search
          </Button>
          {loading && (
            <Button
              onClick={() => {
                active.current?.abort();
                setLoading(false);
              }}
            >
              Stop search
            </Button>
          )}
        </div>
      </form>
      {error && <p role="alert">{error}</p>}
      {progress && (
        <p className={styles.status} role="status">
          {progress}
        </p>
      )}
      {summary && (
        <p className={styles.muted}>
          {summary.hits} matches · {summary.sessions} sessions
          {summary.unavailable
            ? ` · ${summary.unavailable} unavailable projects`
            : ""}
          {summary.pending ? ` · ${summary.pending} pending indexes` : ""}
        </p>
      )}
      {visits.map((visit) => (
        <section className={styles.section} key={visit.sessionId}>
          <strong>{visit.title || visit.alias}</strong>
          <small className={styles.muted}>
            {visit.projectName} · {visit.alias}
            {visit.truncated ? " · Older history was trimmed" : ""}
          </small>
          {visit.hits.map((hit) => (
            <RouteLink
              className={styles.item}
              key={String(hit.seq)}
              to="/sessions/$sessionId"
              params={{ sessionId: visit.sessionId }}
              search={{ seq: hit.seq.toString() }}
            >
              <span>
                {hit.snippet}
                <small>
                  {hit.kind} · <DateTime value={hit.timeMs} />
                </small>
              </span>
            </RouteLink>
          ))}
        </section>
      ))}
      {!loading && summary?.hasMore && (
        <Button onClick={() => void search(true)}>Load more results</Button>
      )}
      {!loading && summary && !visits.length && (
        <p className={styles.muted}>No matches in retained history.</p>
      )}
    </div>
  );
}
