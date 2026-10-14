import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type RefObject,
} from "react";
import { Button, ScrollArea } from "@lesomnus/cxz-ui";
import type {
  SessionSearchHit,
  SessionSearchSummary,
} from "#gen/cxz/session_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./session-search.module.css";

export function SessionSearch({
  c,
  id,
  area,
  jump,
  restoreFocus,
}: {
  c: Connection;
  id: string;
  area: RefObject<HTMLElement | null>;
  jump: (seq: string, inset: number) => void;
  restoreFocus: () => void;
}) {
  useLocale();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [searched, setSearched] = useState("");
  const [hits, setHits] = useState<SessionSearchHit[]>([]);
  const [summary, setSummary] = useState<SessionSearchSummary>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const overlay = useRef<HTMLElement>(null);
  const results = useRef<HTMLDivElement>(null);
  const origin = useRef<HTMLElement | null>(null);
  const active = useRef<AbortController | undefined>(undefined);
  const debounce = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const latest = useRef({ open, query: query.trim(), restoreFocus });
  latest.current = { open, query: query.trim(), restoreFocus };
  const fresh = searched === query.trim();
  const visible = fresh ? hits : [];
  const busy = !!query.trim() && (loading || !fresh);
  function select(hit: SessionSearchHit) {
    const element = overlay.current;
    if (!element) return;
    const bottom = element.getBoundingClientRect().bottom;
    const top = area.current?.getBoundingClientRect().top ?? 0;
    const gap =
      parseFloat(getComputedStyle(element).getPropertyValue("--ui-space")) || 4;
    jump(hit.seq.toString(), bottom - top + 2 * gap);
  }

  function close() {
    active.current?.abort();
    setOpen(false);
    if (
      origin.current?.isConnected &&
      origin.current !== document.body &&
      origin.current !== document.documentElement
    )
      origin.current.focus({ preventScroll: true });
    else latest.current.restoreFocus();
  }
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.isComposing || document.activeElement?.closest("dialog[open]"))
        return;
      if (event.key === "Escape" && latest.current.open) {
        event.preventDefault();
        event.stopImmediatePropagation();
        close();
        return;
      }
      if (
        !event.shiftKey ||
        !(event.ctrlKey || event.metaKey) ||
        event.altKey ||
        event.key.toLowerCase() !== "f"
      )
        return;
      if (
        event.target !== document.body &&
        event.target !== document.documentElement &&
        !area.current?.contains(event.target as Node)
      )
        return;
      event.preventDefault();
      event.stopImmediatePropagation();
      if (!latest.current.open)
        origin.current = document.activeElement as HTMLElement | null;
      setOpen(true);
      input.current?.focus({ preventScroll: true });
      input.current?.select();
    };
    document.addEventListener("keydown", keydown, true);
    return () => document.removeEventListener("keydown", keydown, true);
  }, [area]);
  useEffect(() => {
    if (open) {
      input.current?.focus({ preventScroll: true });
      input.current?.select();
    }
  }, [open]);
  async function search(text: string, cursor = "") {
    clearTimeout(debounce.current);
    if (!latest.current.open || latest.current.query !== text) return;
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    if (!cursor) {
      setHits([]);
      setSummary(undefined);
      setSelected(0);
    }
    setSearched(text);
    setLoading(true);
    setError("");
    try {
      for await (const reply of c.sessions.search(
        {
          query: text,
          match: "substring",
          ignoreCase: true,
          includeTools: false,
          sessions: [id],
          cursor,
          limit: 100,
          snippet: 240,
          clientId: c.clientId,
        },
        { signal: controller.signal, timeoutMs: 60000 },
      )) {
        if (
          controller.signal.aborted ||
          active.current !== controller ||
          latest.current.query !== text
        )
          return;
        if (reply.visit?.sessionId === id) {
          const next = reply.visit.hits;
          setHits((old) => [
            ...new Map([...old, ...next].map((hit) => [hit.seq, hit])).values(),
          ]);
        }
        if (reply.summary) setSummary(reply.summary);
      }
    } catch (error) {
      if (!controller.signal.aborted && active.current === controller)
        setError(String(error));
    } finally {
      if (active.current === controller) setLoading(false);
    }
  }
  useEffect(() => {
    active.current?.abort();
    if (!open || !query.trim()) {
      setLoading(false);
      return;
    }
    debounce.current = setTimeout(() => void search(query.trim()), 250);
    return () => {
      clearTimeout(debounce.current);
      active.current?.abort();
    };
  }, [c, id, open, query]);
  useEffect(() => () => active.current?.abort(), []);
  useEffect(() => {
    results.current
      ?.querySelector<HTMLElement>(`[data-search-index="${selected}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [selected]);
  if (!open) return null;
  return (
    <section
      ref={overlay}
      role="search"
      aria-label={t("Search current conversation")}
      aria-busy={busy}
      className={styles.search}
    >
      <form
        className={styles.controls}
        onSubmit={(event) => {
          event.preventDefault();
          if (!query.trim()) return;
          if (fresh && visible[selected]) select(visible[selected]);
          else void search(query.trim());
        }}
      >
        <svg
          className={styles.icon}
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
          ref={input}
          className={styles.input}
          type="search"
          value={query}
          aria-label={t("Search this conversation")}
          placeholder={t("Search this conversation")}
          autoComplete="off"
          onChange={(event) => {
            setQuery(event.target.value);
            setSelected(0);
          }}
          onKeyDown={(event) => {
            if (event.nativeEvent.isComposing) return;
            if (
              !visible.length ||
              !["ArrowDown", "ArrowUp"].includes(event.key)
            )
              return;
            event.preventDefault();
            setSelected(
              (old) =>
                (old + (event.key === "ArrowDown" ? 1 : -1) + visible.length) %
                visible.length,
            );
          }}
        />
        <Button
          variant="toolbar"
          type="button"
          aria-label={t("Close search")}
          onClick={close}
        >
          ×
        </Button>
      </form>
      {!!query.trim() && (
        <>
          <div className={styles.status} role="status">
            {busy
              ? t("Searching…")
              : error
                ? error
                : visible.length
                  ? t("{count} matches", { count: visible.length })
                  : t("No matches")}
          </div>
          {visible.length > 0 && (
            <ScrollArea
              className={styles.results}
              scrollbarLabel={t("Search results")}
              viewportRef={results}
              style={
                { "--search-result-count": visible.length } as CSSProperties
              }
            >
              {visible.map((hit, index) => (
                <Button
                  key={hit.seq.toString()}
                  className={styles.result}
                  type="button"
                  data-search-index={index}
                  data-seq={hit.seq.toString()}
                  aria-pressed={index === selected}
                  onClick={() => {
                    setSelected(index);
                    select(hit);
                  }}
                >
                  <span className={styles.meta}>
                    {hit.kind === "input" ? ">" : "↳"}
                    {hit.timeMs > 0n && (
                      <time
                        dateTime={new Date(Number(hit.timeMs)).toISOString()}
                      >
                        {new Date(Number(hit.timeMs)).toLocaleString(
                          currentLocale(),
                          {
                            month: "short",
                            day: "numeric",
                            hour: "2-digit",
                            minute: "2-digit",
                          },
                        )}
                      </time>
                    )}
                  </span>
                  <span className={styles.snippet}>{hit.snippet}</span>
                </Button>
              ))}
            </ScrollArea>
          )}
          {fresh && summary?.pending ? (
            <p className={styles.status}>
              {t("Session history is still being indexed.")}
            </p>
          ) : null}
          {fresh && summary?.hasMore && (
            <Button
              type="button"
              className={styles.more}
              disabled={busy}
              onClick={() => void search(query.trim(), summary.nextCursor)}
            >
              {t("Load more results")}
            </Button>
          )}
        </>
      )}
    </section>
  );
}
