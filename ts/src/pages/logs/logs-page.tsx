import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import {
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  Button,
  CheckboxMenu,
  DatePicker,
  Switch,
  FindBar,
  FindBarControls,
  FindBarIcon,
  FindBarInput,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  TableScrollArea,
} from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ResourceSelection } from "#src/shared/api/resource-selection.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { clientLogs, type ClientLogs } from "#src/shared/logs/client-logs.ts";
import type {
  LogCategory,
  LogEntry,
  LogLevel,
  LogQuery,
} from "#src/shared/logs/log-entry.ts";
import {
  ResourceLayout,
  ResourcePage,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { SessionPicker } from "#src/shared/components/session-picker.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import tableStyles from "#src/shared/components/resource-table.module.css";
import { LogDetailsDialog } from "./log-details-dialog";
import { LogLevelIcon } from "./log-level-icon";
import { LogCategoryIcon } from "./log-category-icon";
import { useLogFeed } from "./use-log-feed";
import { useLogViewport } from "./use-log-viewport";
import { logPeriodBounds } from "#src/shared/logs/log-period.ts";
import styles from "./logs-page.module.css";

const logLevels = ["info", "warn", "error"] as const;
const logCategories = ["client", "rpc"] as const;
const emptyProjects: readonly Project[] = [];
const emptySessions: readonly Session[] = [];

export function LogsPage({
  logs: supplied,
  c,
  projects = emptyProjects,
  sessions = emptySessions,
  projectsLoading = false,
  sessionsLoading = false,
}: {
  logs?: ClientLogs;
  c?: Connection;
  projects?: readonly Project[];
  sessions?: readonly Session[];
  projectsLoading?: boolean;
  sessionsLoading?: boolean;
}) {
  const { locale } = useLocale();
  const logs = useMemo(() => supplied ?? clientLogs(), [supplied]);
  const [localSelection] = useState(() => new ResourceSelection());
  const selection = c?.selection ?? localSelection;
  const scope = useSyncExternalStore(selection.subscribe, selection.snapshot);
  const [search, setSearch] = useState("");
  const [period, setPeriod] = useState({ start: "", end: "" });
  const [levels, setLevels] = useState<LogLevel[]>([...logLevels]);
  const [categories, setCategories] = useState<LogCategory[]>([
    ...logCategories,
  ]);
  const filters: LogQuery = useMemo(
    () => ({
      search,
      ...logPeriodBounds(period.start, period.end),
      level: levels,
      category: categories,
      project: scope.project || undefined,
      projectSessions: scope.project
        ? sessions
            .filter(
              (session) =>
                session.project && projectId(session.project) === scope.project,
            )
            .map((session) => sessionUUID(session.id))
        : undefined,
      session: scope.session || undefined,
    }),
    [
      search,
      period,
      levels,
      categories,
      scope.project,
      scope.session,
      sessions,
    ],
  );
  const deferred = useDeferredValue(filters);
  const [live, setLive] = useState(true);
  const feed = useLogFeed(logs, deferred, live);
  const result = feed.page;
  const queryKey = JSON.stringify(deferred);
  const window = useLogViewport(result?.entries.length ?? 0, queryKey);
  const sentinel = useRef<HTMLDivElement>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState("");
  const [selected, setSelected] = useState<LogEntry>();
  const [detailsOpen, setDetailsOpen] = useState(false);
  useEffect(() => {
    if (
      !result?.next ||
      feed.loading ||
      !sentinel.current ||
      !window.viewport.current
    )
      return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) feed.more();
      },
      { root: window.viewport.current, rootMargin: "0px 0px 320px 0px" },
    );
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [result?.next, feed.loading, feed.more, window.viewport]);
  function refresh() {
    if (window.viewport.current) window.viewport.current.scrollTop = 0;
    feed.atTop(true);
    feed.refresh();
  }
  function openDetails(entry: LogEntry) {
    setSelected(entry);
    setDetailsOpen(true);
  }
  async function download() {
    setBusy(true);
    try {
      const text = await logs.export(filters);
      const url = URL.createObjectURL(
        new Blob([text], { type: "application/x-ndjson" }),
      );
      const link = document.createElement("a");
      link.href = url;
      link.download = `cxz-client-logs-${new Date().toISOString().replace(/[:.]/g, "-")}.jsonl`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setActionError("");
    } catch (failure) {
      setActionError(String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function clear() {
    setBusy(true);
    try {
      await logs.clear();
      refresh();
      setActionError("");
    } catch (failure) {
      setActionError(String(failure));
    } finally {
      setBusy(false);
    }
  }
  return (
    <ResourcePage aria-label={t("Logs")}>
      <ResourceToolbar
        aria-label={t("Logs")}
        actions={
          <>
            <ToolbarIconButton
              icon="refresh"
              label={t("Refresh")}
              onClick={refresh}
            />
            <ToolbarIconButton
              icon="download"
              label={t("Download filtered logs")}
              disabled={busy}
              onClick={download}
            />
            <ToolbarIconButton
              icon="erase"
              label={t("Clear logs")}
              disabled={busy}
              onClick={clear}
            />
          </>
        }
      >
        <ProjectPicker
          projects={projects}
          value={scope.project}
          allowAll
          loading={projectsLoading}
          choose={(project) => selection.chooseProject(project)}
        />
        <SessionPicker
          sessions={sessions}
          project={scope.project || undefined}
          value={scope.session}
          loading={sessionsLoading}
          choose={(id) => {
            const session = sessions.find(
              (session) => sessionUUID(session.id) === id,
            );
            selection.chooseSession(
              session?.project ? projectId(session.project) : scope.project,
              id,
              session?.runtimeId ?? "",
            );
          }}
        />
        <span className={styles.live}>
          <span>{t("Live updates")}</span>
          <Switch
            label={t("Live updates")}
            checked={live}
            onChange={(value) => {
              setLive(value);
              if (value) refresh();
            }}
          />
        </span>
      </ResourceToolbar>
      <div className={`${pageStyles.contentPadding} ${styles.body}`}>
        <ResourceLayout className={styles.content}>
          <FindBar aria-label={t("Search logs")}>
            <FindBarControls className={styles.searchControls}>
              <FindBarIcon />
              <FindBarInput
                className={styles.searchInput}
                aria-label={t("Search logs")}
                placeholder={t("Search logs")}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
              <div className={styles.filters}>
                <div
                  className={styles.period}
                  role="group"
                  aria-label={t("Log period (local time)")}
                >
                  <DatePicker
                    variant="toolbar"
                    className={styles.datePicker}
                    label={t("Log start date")}
                    placeholder={t("From")}
                    value={period.start || null}
                    max={period.end || undefined}
                    locale={locale}
                    formatDate={(value) => value}
                    onChange={(value) =>
                      setPeriod((current) => ({
                        ...current,
                        start: value ?? "",
                      }))
                    }
                  />
                  <span className={styles.dateSeparator} aria-hidden="true">
                    –
                  </span>
                  <DatePicker
                    variant="toolbar"
                    className={styles.datePicker}
                    label={t("Log end date")}
                    placeholder={t("Through")}
                    value={period.end || null}
                    min={period.start || undefined}
                    locale={locale}
                    formatDate={(value) => value}
                    onChange={(value) =>
                      setPeriod((current) => ({ ...current, end: value ?? "" }))
                    }
                  />
                </div>
                <CheckboxMenu
                  label={t("Log level")}
                  allLabel={t("All levels")}
                  triggerContent={t("Level")}
                  options={logLevels.map((level) => ({
                    value: level,
                    label: level,
                    icon: <LogLevelIcon level={level} />,
                  }))}
                  value={levels}
                  choose={(values) =>
                    setLevels(
                      logLevels.filter((level) => values.includes(level)),
                    )
                  }
                />
                <CheckboxMenu
                  label={t("Log category")}
                  allLabel={t("All categories")}
                  triggerContent={t("Category")}
                  options={logCategories.map((category) => ({
                    value: category,
                    label: category === "rpc" ? "RPC" : "Client",
                    icon: <LogCategoryIcon category={category} />,
                  }))}
                  value={categories}
                  choose={(values) =>
                    setCategories(
                      logCategories.filter((category) =>
                        values.includes(category),
                      ),
                    )
                  }
                />
              </div>
            </FindBarControls>
          </FindBar>
          <div className={styles.status} role="status">
            {result ? (
              <>
                <span>
                  {result.storage === "opfs" ? "OPFS" : t("Memory only")}
                </span>
                <span>
                  {(result.bytes / 1024 / 1024).toFixed(2)} /{" "}
                  {result.capacity / 1024 / 1024} MiB
                </span>
                <span>
                  {t("{count} matching records", { count: result.total })}
                </span>
              </>
            ) : (
              t("Loading…")
            )}
          </div>
          {result?.storage === "memory" && (
            <p className={styles.notice}>
              {t(
                "Persistent log storage is unavailable. These logs are kept in this tab and disappear on reload (up to 2 MiB).",
              )}
            </p>
          )}
          {(actionError || feed.error) && (
            <ErrorNotice error={actionError || feed.error} />
          )}
          <TableScrollArea
            className={styles.scroll}
            viewportRef={window.viewport}
            data-loaded-count={result?.entries.length ?? 0}
            scrollbarLabel={t("Logs")}
            onScroll={(event) => {
              feed.atTop(event.currentTarget.scrollTop < 2);
              window.scrolled();
            }}
          >
            <Table
              className={`${tableStyles.layout} ${tableStyles.table} ${styles.table}`}
              aria-label={t("Client logs")}
              aria-rowcount={result ? result.total + 1 : undefined}
            >
              <colgroup>
                <col className={styles.timeColumn} />
                <col className={styles.levelColumn} />
                <col className={styles.categoryColumn} />
                <col />
              </colgroup>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Time")}</TableHead>
                  <TableHead aria-label={t("Level")} />
                  <TableHead aria-label={t("Category")} />
                  <TableHead>{t("Message")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {window.before > 0 && <Spacer height={window.before} />}
                {result?.entries
                  .slice(window.start, window.end)
                  .map((entry, index) => (
                    <TableRow
                      key={entry.id}
                      data-log-row
                      aria-rowindex={window.start + index + 2}
                      className={styles.row}
                      selected={detailsOpen && selected?.id === entry.id}
                      onClick={() => openDetails(entry)}
                    >
                      <TableCell className={styles.time}>
                        <DateTime value={entry.time} showOffset={false} />
                      </TableCell>
                      <TableCell align="center">
                        <LogLevelIcon level={entry.level} />
                      </TableCell>
                      <TableCell align="center">
                        <LogCategoryIcon category={entry.category} />
                      </TableCell>
                      <TableCell className={styles.messageCell}>
                        <Button
                          className={styles.message}
                          aria-label={entry.message}
                          aria-haspopup="dialog"
                          onClick={(event) => {
                            event.stopPropagation();
                            openDetails(entry);
                          }}
                        >
                          <span>{entry.message}</span>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                {window.after > 0 && <Spacer height={window.after} />}
              </TableBody>
            </Table>
            {result && !result.entries.length && (
              <p className={styles.empty}>{t("No matching logs")}</p>
            )}
            <div
              ref={sentinel}
              className={styles.sentinel}
              aria-busy={feed.loading}
            >
              {feed.loading && result?.entries.length
                ? t("Loading more logs…")
                : null}
            </div>
          </TableScrollArea>
        </ResourceLayout>
      </div>
      {selected && (
        <LogDetailsDialog
          entry={selected}
          open={detailsOpen}
          close={() => setDetailsOpen(false)}
          closed={() => setSelected(undefined)}
        />
      )}
    </ResourcePage>
  );
}
function Spacer({ height }: { height: number }) {
  return (
    <TableRow className={styles.spacer} aria-hidden="true">
      <TableCell colSpan={4} style={{ height }} />
    </TableRow>
  );
}
