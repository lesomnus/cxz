import { useDeferredValue, useEffect, useMemo, useState } from "react";
import {
  Button,
  Switch,
  ValueMenu,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  TableScrollArea,
} from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { clientLogs, type ClientLogs } from "#src/shared/logs/client-logs.ts";
import {
  pageSize,
  type LogPage,
  type LogQuery,
} from "#src/shared/logs/log-entry.ts";
import styles from "./logs-page.module.css";

export function LogsPage({ logs: supplied }: { logs?: ClientLogs }) {
  useLocale();
  const logs = useMemo(() => supplied ?? clientLogs(), [supplied]);
  const [filters, setFilters] = useState<LogQuery>({});
  const deferred = useDeferredValue(filters);
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [live, setLive] = useState(true);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<LogPage>();
  const [error, setError] = useState("");
  const query = useMemo(() => ({ ...deferred, offset }), [deferred, offset]);
  useEffect(() => {
    if (!live) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const unsubscribe = logs.subscribe(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        setRevision((r) => r + 1);
      }, 200);
    });
    return () => {
      clearTimeout(timer);
      unsubscribe();
    };
  }, [logs, live]);
  useEffect(() => {
    let disposed = false;
    logs
      .query(query)
      .then((page) => {
        if (!disposed) {
          setResult(page);
          setError("");
        }
      })
      .catch((failure) => {
        if (!disposed) setError(String(failure));
      });
    return () => {
      disposed = true;
    };
  }, [logs, query, revision]);
  function filter(key: keyof LogQuery, value: string) {
    setOffset(0);
    setFilters((old) => ({ ...old, [key]: value }));
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
    } catch (failure) {
      setError(String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function clear() {
    setBusy(true);
    try {
      await logs.clear();
      setOffset(0);
      setRevision((r) => r + 1);
      setError("");
    } catch (failure) {
      setError(String(failure));
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className={styles.page} aria-label={t("Logs")}>
      <header className={styles.header}>
        <div>
          <h1>{t("Logs")}</h1>
          <small>{t("Diagnostics observed by this browser")}</small>
        </div>
        <div className={styles.actions}>
          <Switch
            label={t("Live updates")}
            checked={live}
            onChange={(value) => {
              setLive(value);
              setOffset(0);
              setRevision((r) => r + 1);
            }}
          />
          <Button onClick={() => setRevision((r) => r + 1)}>
            {t("Refresh")}
          </Button>
          <Button disabled={busy} onClick={download}>
            {t("Download filtered logs")}
          </Button>
          <Button disabled={busy} onClick={clear}>
            {t("Clear logs")}
          </Button>
        </div>
      </header>
      <div className={styles.filters}>
        <input
          type="search"
          aria-label={t("Search logs")}
          placeholder={t("Search logs")}
          value={filters.search ?? ""}
          onChange={(e) => filter("search", e.target.value)}
        />
        <ValueMenu
          label={t("Log level")}
          value={filters.level ?? ""}
          choose={(value) => filter("level", value)}
          options={[
            { value: "", label: t("All levels") },
            { value: "info", label: "Info" },
            { value: "warn", label: "Warn" },
            { value: "error", label: "Error" },
          ]}
        />
        <ValueMenu
          label={t("Log category")}
          value={filters.category ?? ""}
          choose={(value) => filter("category", value)}
          options={[
            { value: "", label: t("All categories") },
            { value: "rpc", label: "RPC" },
            { value: "client", label: "Client" },
          ]}
        />
        {(
          [
            ["connection", t("Connection URL")],
            ["project", t("Project UUID")],
            ["session", t("Session UUID")],
          ] as const
        ).map(([key, label]) => (
          <input
            key={key}
            aria-label={label}
            placeholder={label}
            value={filters[key] ?? ""}
            onChange={(e) => filter(key, e.target.value.trim())}
          />
        ))}
      </div>
      <div className={styles.status} role="status">
        {result ? (
          <>
            <span>{result.storage === "opfs" ? "OPFS" : t("Memory only")}</span>
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
      {error && <p role="alert">{error}</p>}
      <TableScrollArea className={styles.scroll} scrollbarLabel={t("Logs")}>
        <Table className={styles.table} aria-label={t("Client logs")}>
          <TableHeader>
            <TableRow>
              <TableHead>{t("Time")}</TableHead>
              <TableHead>{t("Level")}</TableHead>
              <TableHead>{t("Message")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {result?.entries.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell>
                  <DateTime value={entry.time} />
                </TableCell>
                <TableCell>
                  <span className={styles.level} data-level={entry.level}>
                    {entry.level}
                  </span>
                </TableCell>
                <TableCell>
                  <details>
                    <summary>
                      <span>{entry.message}</span>
                      <small>{entry.method ?? entry.category}</small>
                    </summary>
                    <pre>{JSON.stringify(entry, null, 2)}</pre>
                  </details>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {result && !result.entries.length && (
          <p className={styles.empty}>{t("No matching logs")}</p>
        )}
      </TableScrollArea>
      <footer className={styles.footer}>
        <Button
          disabled={!offset}
          onClick={() => {
            setLive(false);
            setOffset(Math.max(0, offset - pageSize));
          }}
        >
          {t("Previous")}
        </Button>
        <span>
          {result?.total
            ? `${offset + 1}–${Math.min(offset + pageSize, result.total)} / ${result.total}`
            : "0"}
        </span>
        <Button
          disabled={!result || offset + pageSize >= result.total}
          onClick={() => {
            setLive(false);
            setOffset(offset + pageSize);
          }}
        >
          {t("Next")}
        </Button>
      </footer>
    </main>
  );
}
