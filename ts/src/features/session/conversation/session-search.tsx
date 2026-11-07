import type { EventListSource } from "#src/features/session/model/event-source.ts";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type RefObject,
} from "react";
import {
  DateTime,
  Popover,
  FindBar,
  FindBarControls,
  FindBarIcon,
  FindBarInput,
  FindBarCounter,
  FindBarOptions,
  FindBarClose,
  FindBarResults,
  FindBarResult,
  FindBarMessage,
} from "@lesomnus/nox";
import { searchLoadedMessages, type LoadedSearchHit } from "./loaded-search";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./session-search.module.css";
import { useSearchHighlights, type SearchLocation } from "./search-highlights";
import { messageDate, relativeMessageTime } from "./message-time";
import {
  compileSearchPattern,
  defaultSearchOptions,
  type SearchOptions,
} from "./search-pattern";

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
  const [options, setOptions] = useState(defaultSearchOptions);
  const errorId = useId();
  const [selected, setSelected] = useState(0);
  const [location, setLocation] = useState<SearchLocation>();
  const [now, setNow] = useState(() => new Date());
  const position = useRef<SearchLocation | undefined>(undefined);
  const navigation = useRef(0);
  const input = useRef<HTMLInputElement>(null);
  const overlay = useRef<HTMLElement>(null);
  const results = useRef<HTMLDivElement>(null);
  const origin = useRef<HTMLElement | null>(null);
  const latest = useRef({ open, restoreFocus });
  latest.current = { open, restoreFocus };
  const searchQuery = options.useRegex ? query : query.trim();
  const compiled = useMemo(
    () => compileSearchPattern(searchQuery, options),
    [searchQuery, options],
  );
  const pattern = open ? compiled.pattern : undefined;
  const visible = useMemo(
    () => (pattern ? searchLoadedMessages(events, pattern) : noHits),
    [events, pattern],
  );
  const selectedIndex = Math.min(selected, Math.max(0, visible.length - 1));
  useEffect(() => {
    if (!open) return;
    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 30_000);
    return () => clearInterval(timer);
  }, [open]);
  useEffect(() => {
    onMatchesChange?.(visible.map((hit) => hit.seq.toString()));
  }, [visible, onMatchesChange]);
  useEffect(() => () => onMatchesChange?.([]), [onMatchesChange]);
  const matches = useSearchHighlights({
    area,
    pattern,
    location,
    located: (occurrence) => {
      if (position.current)
        position.current = { ...position.current, occurrence };
      input.current?.focus({ preventScroll: true });
    },
  });
  function resetNavigation() {
    navigation.current++;
    position.current = undefined;
    setLocation(undefined);
    setSelected(0);
  }
  function changeOption(key: keyof SearchOptions, pressed: boolean) {
    resetNavigation();
    setOptions((current) => ({ ...current, [key]: pressed }));
    input.current?.focus({ preventScroll: true });
  }
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
    <FindBar
      motion
      ref={overlay}
      role="search"
      aria-label={t("Search current conversation")}
      className={styles.search}
    >
      <FindBarControls>
        <FindBarIcon />
        <FindBarInput
          ref={input}
          type="search"
          value={query}
          aria-label={t("Search this conversation")}
          aria-invalid={!!compiled.error || undefined}
          aria-describedby={compiled.error ? errorId : undefined}
          placeholder={t("Search this conversation")}
          autoComplete="off"
          onChange={(event) => {
            resetNavigation();
            setQuery(event.target.value);
          }}
          onKeyDown={(event) => {
            if (event.nativeEvent.isComposing) return;
            if (event.key === "Enter") {
              event.preventDefault();
              void nextMatch();
              return;
            }
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
        <FindBarCounter
          current={visible.length ? selectedIndex + 1 : 0}
          count={visible.length}
        />
        <FindBarOptions
          options={[
            {
              id: "matchCase",
              label: t("Match case"),
              symbol: "Aa",
              pressed: options.matchCase,
            },
            {
              id: "wholeWord",
              label: t("Match whole word"),
              symbol: "ab",
              pressed: options.wholeWord,
            },
            {
              id: "useRegex",
              label: t("Use regular expression"),
              symbol: ".*",
              pressed: options.useRegex,
            },
          ]}
          onChange={(id, pressed) =>
            changeOption(id as keyof SearchOptions, pressed)
          }
        />
        <FindBarClose onClick={close} />
      </FindBarControls>
      {!!searchQuery && (
        <>
          {compiled.error ? (
            <FindBarMessage id={errorId} role="alert">
              <Popover content={compiled.error}>
                {t("Invalid regular expression")}
              </Popover>
            </FindBarMessage>
          ) : (
            visible.length === 0 && (
              <FindBarMessage>{t("No matches")}</FindBarMessage>
            )
          )}
          {visible.length > 0 && (
            <FindBarResults
              scrollbarOffset={5}
              scrollbarLabel={t("Search results")}
              viewportRef={results}
              style={
                { "--search-result-count": visible.length } as CSSProperties
              }
            >
              {visible.map((hit, index) => {
                const date = messageDate(hit.timeMs);
                const relative = date ? relativeMessageTime(date, now) : "";
                return (
                  <FindBarResult
                    key={hit.seq.toString()}
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
                      {date && <DateTime value={date} />}
                      {relative && (
                        <span className={styles.relative}>- {relative}</span>
                      )}
                    </span>
                    <SearchSnippet hit={hit} />
                  </FindBarResult>
                );
              })}
            </FindBarResults>
          )}
        </>
      )}
    </FindBar>
  );
}

function SearchSnippet({ hit }: { hit: LoadedSearchHit }) {
  const content: ReactNode[] = [];
  let offset = 0;
  for (const { start, end } of hit.snippetMatches) {
    content.push(
      hit.snippet.slice(offset, start),
      <mark key={start}>{hit.snippet.slice(start, end)}</mark>,
    );
    offset = end;
  }
  content.push(hit.snippet.slice(offset));
  return <span className={styles.snippet}>{content}</span>;
}
