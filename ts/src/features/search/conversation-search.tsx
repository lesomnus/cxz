import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { useEffect, useRef, useState } from "react";
import {
  Button,
  ToggleButton,
  Popover,
  PrefixValueMenu,
  DatePicker,
} from "@lesomnus/nox";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import type {
  SessionSearchVisit,
  SessionSearchSummary,
} from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import styles from "./conversation-search.module.css";
import bar from "#src/shared/components/search-bar.module.css";

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
    setVisits([]);
    setProgress("");
    setError("");
  }
  async function search(more = false) {
    if (!more && !query.trim()) return;
    if (!more && since && until && since >= until) {
      setError("Choose an end date after the start date.");
      return;
    }
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
        role="search"
        aria-label="Search conversations"
        onSubmit={(event) => {
          event.preventDefault();
          if (!loading) void search();
        }}
      >
        <div className={bar.surface}>
          <div className={bar.controls}>
            <svg
              className={bar.icon}
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              aria-hidden="true"
            >
              <circle cx="10" cy="10" r="6" />
              <path d="m15 15 5 5" />
            </svg>
            <input
              className={bar.input}
              type="search"
              aria-label="Search conversations"
              placeholder="Search conversations"
              autoComplete="off"
              value={query}
              onChange={(event) => {
                invalidate();
                setQuery(event.target.value);
              }}
            />
            <div className={bar.modes} role="group" aria-label="Search options">
              <Popover content="Match case">
                <ToggleButton
                  className={bar.mode}
                  variant="toolbar"
                  aria-label="Match case"
                  pressed={!ignoreCase}
                  onPressedChange={(pressed) => {
                    invalidate();
                    setIgnoreCase(!pressed);
                  }}
                >
                  <span aria-hidden="true">Aa</span>
                </ToggleButton>
              </Popover>
              <Popover content="Use regular expression">
                <ToggleButton
                  className={bar.mode}
                  variant="toolbar"
                  aria-label="Use regular expression"
                  pressed={match === "regex"}
                  onPressedChange={(pressed) => {
                    invalidate();
                    setMatch(pressed ? "regex" : "substring");
                  }}
                >
                  <span aria-hidden="true">.*</span>
                </ToggleButton>
              </Popover>
              <Popover content="Fuzzy search">
                <ToggleButton
                  className={bar.mode}
                  variant="toolbar"
                  aria-label="Fuzzy search"
                  pressed={match === "fuzzy"}
                  onPressedChange={(pressed) => {
                    invalidate();
                    setMatch(pressed ? "fuzzy" : "substring");
                  }}
                >
                  <span aria-hidden="true">≈</span>
                </ToggleButton>
              </Popover>
            </div>
            {loading ? (
              <Button
                className={bar.action}
                variant="toolbar"
                type="button"
                aria-label="Stop search"
                title="Stop search"
                onClick={(event) => {
                  event.preventDefault();
                  active.current?.abort();
                  setLoading(false);
                  setProgress("Search stopped.");
                }}
              >
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 16 16"
                  fill="currentColor"
                  aria-hidden="true"
                >
                  <rect x="4" y="4" width="8" height="8" rx="1" />
                </svg>
              </Button>
            ) : (
              <Button
                className={bar.action}
                variant="toolbar"
                type="submit"
                aria-label="Search"
                title="Search"
                disabled={!query.trim()}
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.6"
                  aria-hidden="true"
                >
                  <path d="M5 12h14m-5-5 5 5-5 5" />
                </svg>
              </Button>
            )}
          </div>
        </div>
        <div className={styles.filterBar}>
          <PrefixValueMenu
            prefix={<ResourceIcon kind="projects" />}
            label="Search project"
            value={project}
            className={styles.projectPicker}
            display={
              projects.find((p) => projectId(p) === project)?.name ||
              projects.find((p) => projectId(p) === project)?.alias ||
              "All projects"
            }
            options={[
              { value: "", label: "All projects" },
              ...projects.map((p) => ({
                value: projectId(p),
                label: p.name || p.alias,
                searchText: `${p.alias} ${p.workspace}`,
              })),
            ]}
            searchable
            choose={(value) => {
              invalidate();
              setProject(value);
            }}
          />
          <div
            className={styles.dateRange}
            role="group"
            aria-label="Date range (UTC, end exclusive)"
          >
            <DatePicker
              label="From (UTC)"
              name="since"
              placeholder="From"
              className={styles.datePicker}
              value={since || null}
              onChange={(value) => {
                invalidate();
                setSince(value ?? "");
              }}
            />
            <span className={styles.dateSeparator} aria-hidden="true">
              –
            </span>
            <DatePicker
              label="Until (UTC, exclusive)"
              name="until"
              placeholder="Until"
              className={styles.datePicker}
              value={until || null}
              onChange={(value) => {
                invalidate();
                setUntil(value ?? "");
              }}
            />
          </div>
          {(project || since || until) && (
            <Button
              variant="toolbar"
              type="button"
              onClick={() => {
                invalidate();
                setProject("");
                setSince("");
                setUntil("");
              }}
            >
              Reset filters
            </Button>
          )}
        </div>
      </form>
      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}
      {progress && (
        <p className={styles.status} role="status">
          {progress}
        </p>
      )}
      {summary && (
        <p className={styles.summary} role="status">
          {summary.hits} matches · {summary.sessions} sessions
          {summary.unavailable
            ? ` · ${summary.unavailable} unavailable projects`
            : ""}
          {summary.pending ? ` · ${summary.pending} pending indexes` : ""}
        </p>
      )}
      {visits.map((visit) => (
        <section className={styles.section} key={visit.sessionId}>
          <header className={styles.resultHeading}>
            <strong>{visit.title || visit.alias}</strong>
            <small className={styles.muted}>
              {visit.projectName} · {visit.alias}
              {visit.truncated ? " · Older history was trimmed" : ""}
            </small>
          </header>
          {visit.hits.map((hit) => (
            <RouteLink
              className={styles.item}
              key={String(hit.seq)}
              {...sessionLocation(visit.alias || visit.sessionId)}
              search={{ seq: hit.seq.toString() }}
            >
              <span className={styles.snippet}>
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
