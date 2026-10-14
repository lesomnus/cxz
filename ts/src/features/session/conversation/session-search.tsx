import type { EventListSource } from "#src/features/session/model/event-source.ts";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type RefObject,
} from "react";
import { Button, ScrollArea } from "@lesomnus/nox";
import { searchLoadedMessages, type LoadedSearchHit } from "./loaded-search";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./session-search.module.css";
import { useSearchHighlights, type SearchLocation } from "./search-highlights";

const noHits: LoadedSearchHit[] = [];

export function SessionSearch({
  source,
  area,
  jump,
  restoreFocus,
  onMatchesChange,
}: {
  source: EventListSource;
  area: RefObject<HTMLElement | null>;
  jump: (
    seq: string,
    inset: number,
    matchVisible?: boolean,
  ) => void | Promise<void>;
  restoreFocus: () => void;
  onMatchesChange?: (seqs: readonly string[]) => void;
}) {
  const events = source();
  useLocale();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const [location, setLocation] = useState<SearchLocation>();
  const position = useRef<SearchLocation | undefined>(undefined);
  const navigation = useRef(0);
  const input = useRef<HTMLInputElement>(null);
  const overlay = useRef<HTMLElement>(null);
  const results = useRef<HTMLDivElement>(null);
  const origin = useRef<HTMLElement | null>(null);
  const latest = useRef({ open, query: query.trim(), restoreFocus });
  latest.current = { open, query: query.trim(), restoreFocus };
  const visible = useMemo(
    () =>
      open && query.trim()
        ? searchLoadedMessages(events, query.trim())
        : noHits,
    [events, open, query],
  );
  const selectedIndex = Math.min(selected, Math.max(0, visible.length - 1));
  useEffect(() => {
    onMatchesChange?.(visible.map((hit) => hit.seq.toString()));
  }, [visible, onMatchesChange]);
  useEffect(() => () => onMatchesChange?.([]), [onMatchesChange]);
  const matches = useSearchHighlights({
    area,
    query: open ? query.trim() : "",
    location,
    located: (occurrence) => {
      if (position.current)
        position.current = { ...position.current, occurrence };
      input.current?.focus({ preventScroll: true });
    },
  });
  async function select(hit: LoadedSearchHit, occurrence = -1) {
    const element = overlay.current;
    if (!element) return;
    const bottom = element.getBoundingClientRect().bottom;
    const top = area.current?.getBoundingClientRect().top ?? 0;
    const gap =
      parseFloat(getComputedStyle(element).getPropertyValue("--ui-space")) || 4;
    const next = {
      seq: hit.seq.toString(),
      occurrence: occurrence < 0 ? hit.occurrences - 1 : occurrence,
      inset: bottom - top + 2 * gap,
      token: ++navigation.current,
    };
    position.current = next;
    // An already mounted match only needs exact Range navigation. Avoid an
    // intermediate jump to the message header and redundant history loading.
    await jump(next.seq, next.inset, matches.current.has(next.seq));
    if (!latest.current.open || next.token !== navigation.current) return;
    setLocation(next);
    requestAnimationFrame(() => input.current?.focus({ preventScroll: true }));
  }

  async function nextMatch() {
    if (!query.trim()) return;
    const available = visible;
    if (!available.length) return;
    const current = position.current;
    const index = available.findIndex(
      (hit) => hit.seq.toString() === current?.seq,
    );
    if (current && index === selectedIndex) {
      if (current.occurrence > 0) {
        await select(available[index], current.occurrence - 1);
        return;
      }
      const next = (index + 1) % available.length;
      setSelected(next);
      await select(available[next]);
    } else await select(available[selectedIndex] ?? available[0]);
  }

  function close() {
    navigation.current++;
    position.current = undefined;
    setLocation(undefined);
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
  useEffect(() => {
    const current = position.current;
    const index = visible.findIndex(
      (hit) => hit.seq.toString() === current?.seq,
    );
    if (current && index < 0) {
      position.current = undefined;
      setLocation(undefined);
    }
    setSelected(index >= 0 ? index : 0);
  }, [visible]);
  useEffect(() => {
    results.current
      ?.querySelector<HTMLElement>(`[data-search-index="${selectedIndex}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);
  if (!open) return null;
  return (
    <section
      ref={overlay}
      role="search"
      aria-label={t("Search current conversation")}
      className={styles.search}
    >
      <form
        className={styles.controls}
        onSubmit={(event) => {
          event.preventDefault();
          void nextMatch();
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
            navigation.current++;
            position.current = undefined;
            setLocation(undefined);
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
            {visible.length
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
                  aria-pressed={index === selectedIndex}
                  onClick={() => {
                    setSelected(index);
                    void select(hit);
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
        </>
      )}
    </section>
  );
}
