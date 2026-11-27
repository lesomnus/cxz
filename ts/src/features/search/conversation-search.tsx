import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useResourceRoutes } from "#src/shared/navigation/resource-route.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { useEffect, useRef, useState } from "react";
import {
  Button,
  FindBar,
  FindBarControls,
  FindBarIcon,
  FindBarInput,
  FindBarOptions,
  FindBarAction,
  PrefixValueMenu,
  DatePicker,
} from "@lesomnus/nox";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { projectIconName } from "#src/shared/components/project-icon.ts";
import type {
  SessionSearchVisit,
  SessionSearchSummary,
} from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  searchHistoryLifetime,
  type RecentSearch,
  type SearchFilters,
  type SearchMatch,
} from "./recent-searches";
import { RecentSearchTable } from "./recent-search-table";
import styles from "./conversation-search.module.css";
import inputSurface from "#src/shared/components/input-surface.module.css";

function recentWeek() {
  const end = new Date();
  end.setUTCHours(24, 0, 0, 0);
  const start = new Date(end.getTime() - 7 * 86400000);
  return {
    since: start.toISOString().slice(0, 10),
    until: end.toISOString().slice(0, 10),
  };
}

export function ConversationSearch({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const { sessionLocation } = useResourceRoutes();
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState<SearchMatch>("substring");
  const [project, setProject] = useState("");
  const [ignoreCase, setIgnoreCase] = useState(true);
  const [{ since, until }, setDates] = useState(recentWeek);
  const [visits, setVisits] = useState<SessionSearchVisit[]>([]);
  const [summary, setSummary] = useState<SessionSearchSummary>();
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [recent, setRecent] = useState(() => c.searchHistory.list());
  const active = useRef<AbortController | undefined>(undefined);
  const submitted = useRef<RecentSearch | undefined>(undefined);
  const selectedProject = projects.find((p) => projectId(p) === project);
  useEffect(() => () => active.current?.abort(), []);
  useEffect(() => {
    const oldest = recent.at(-1);
    if (!oldest) return;
    const timer = setTimeout(
      () => setRecent(c.searchHistory.list()),
      Math.max(0, oldest.time + searchHistoryLifetime - Date.now()),
    );
    return () => clearTimeout(timer);
  }, [c, recent]);
  function invalidate() {
    active.current?.abort();
    setLoading(false);
    setSummary(undefined);
    setVisits([]);
    setProgress("");
    setError("");
  }
  async function search(
    more = false,
    filters: SearchFilters = {
      query,
      project,
      projectName: selectedProject?.name || selectedProject?.alias || "",
      match,
      ignoreCase,
      since,
      until,
    },
  ) {
    const run = more
      ? submitted.current
      : {
          ...filters,
          id: crypto.randomUUID(),
          time: Date.now(),
          hits: 0,
          hasMore: false,
          partial: false,
        };
    if (!run || !run.query.trim()) return;
    if (!more && run.since && run.until && run.since >= run.until) {
      setError("Choose an end date after the start date.");
      return;
    }
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    if (!more) {
      setVisits([]);
      setSummary(undefined);
    }
    submitted.current = run;
    const stamp = (date: string) =>
      date
        ? {
            seconds: BigInt(Math.floor(Date.parse(date + "T00:00:00Z") / 1000)),
            nanos: 0,
          }
        : undefined;
    setLoading(true);
    setError("");
    setProgress("");
    try {
      for await (const reply of c.sessions.search(
        {
          query: run.query,
          match: run.match,
          ignoreCase: run.ignoreCase,
          projects: run.project ? [run.project] : [],
          since: stamp(run.since),
          until: stamp(run.until),
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
        if (reply.summary) {
          setSummary(reply.summary);
          const completed = {
            ...run,
            hits: run.hits + reply.summary.hits,
            hasMore: reply.summary.hasMore,
            partial:
              run.partial ||
              reply.summary.pending > 0 ||
              reply.summary.unavailable > 0,
          };
          submitted.current = completed;
          setRecent(c.searchHistory.remember(completed, more));
        }
      }
    } catch (error) {
      if (!controller.signal.aborted) setError(String(error));
    } finally {
      if (active.current === controller) setLoading(false);
    }
  }
  function repeat(previous: RecentSearch) {
    setQuery(previous.query);
    setMatch(previous.match);
    setIgnoreCase(previous.ignoreCase);
    setProject(previous.project);
    setDates({ since: previous.since, until: previous.until });
    void search(false, previous);
  }
  return (
    <div className={styles.content}>
      <form
        className={styles.searchForm}
        role="search"
        aria-label="Search conversations"
        onSubmit={(event) => {
          event.preventDefault();
          if (!loading) void search();
        }}
      >
        <FindBar
          className={`${styles.searchBar} ${inputSurface.surface}`}
          aria-label="Search conversations"
        >
          <FindBarControls>
            <FindBarIcon />
            <FindBarInput
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
            <FindBarOptions
              options={[
                {
                  id: "matchCase",
                  label: "Match case",
                  symbol: "Aa",
                  pressed: !ignoreCase,
                },
                {
                  id: "regex",
                  label: "Use regular expression",
                  symbol: ".*",
                  pressed: match === "regex",
                },
                {
                  id: "fuzzy",
                  label: "Fuzzy search",
                  symbol: "≈",
                  pressed: match === "fuzzy",
                },
              ]}
              onChange={(id, pressed) => {
                invalidate();
                if (id === "matchCase") setIgnoreCase(!pressed);
                else if (id === "regex" || id === "fuzzy")
                  setMatch(pressed ? id : "substring");
              }}
            />
            {loading ? (
              <FindBarAction
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
                <Icon name="stop" width="14" height="14" aria-hidden="true" />
              </FindBarAction>
            ) : (
              <FindBarAction
                variant="toolbar"
                type="submit"
                aria-label="Search"
                title="Search"
                disabled={!query.trim()}
              >
                <Icon
                  name="run-search"
                  width="16"
                  height="16"
                  aria-hidden="true"
                />
              </FindBarAction>
            )}
          </FindBarControls>
        </FindBar>
        <div
          className={styles.filterBar}
          role="group"
          aria-label="Search filters"
        >
          <PrefixValueMenu
            prefix={
              <ResourceIcon
                kind="projects"
                icon={projectIconName(selectedProject)}
              />
            }
            label="Search project"
            value={project}
            className={styles.projectPicker}
            display={
              selectedProject?.name ||
              selectedProject?.alias ||
              project ||
              "All projects"
            }
            options={[
              { value: "", label: "All projects" },
              ...projects.map((p) => ({
                value: projectId(p),
                label: p.name || p.alias,
                icon: (
                  <ResourceIcon kind="projects" icon={projectIconName(p)} />
                ),
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
              variant="toolbar"
              label="From (UTC)"
              name="since"
              placeholder="From"
              className={styles.datePicker}
              value={since || null}
              formatDate={(value) => value}
              onChange={(value) => {
                invalidate();
                setDates((old) => ({ ...old, since: value ?? "" }));
              }}
            />
            <span className={styles.dateSeparator} aria-hidden="true">
              –
            </span>
            <DatePicker
              variant="toolbar"
              label="Until (UTC, exclusive)"
              name="until"
              placeholder="Until"
              className={styles.datePicker}
              value={until || null}
              formatDate={(value) => value}
              onChange={(value) => {
                invalidate();
                setDates((old) => ({ ...old, until: value ?? "" }));
              }}
            />
          </div>
        </div>
      </form>
      {error && <ErrorNotice className={styles.error} error={error} />}
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
      {!loading && !summary && !visits.length && recent.length > 0 && (
        <RecentSearchTable
          recent={recent}
          repeat={repeat}
          clear={() => setRecent(c.searchHistory.clear())}
        />
      )}
    </div>
  );
}
