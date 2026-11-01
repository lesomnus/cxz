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
  Button,
  CompactNumber,
  DateTime,
  Popover,
  ScrollArea,
  ToggleButton,
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
        <div
          className={styles.counter}
          role="status"
          aria-label={t("Result {current} of {count}", {
            current: visible.length ? selectedIndex + 1 : 0,
            count: visible.length,
          })}
        >
          <span aria-hidden="true">
            <CompactNumber
              value={visible.length ? selectedIndex + 1 : 0}
              mode="integer"
            />
            <span className={styles.separator}>/</span>
            <CompactNumber value={visible.length} mode="integer" />
          </span>
        </div>
        <div
          className={styles.modes}
          role="group"
          aria-label={t("Search options")}
        >
          <Popover content={t("Match case")}>
            <ToggleButton
              className={styles.mode}
              variant="toolbar"
              aria-label={t("Match case")}
              pressed={options.matchCase}
              onPressedChange={(pressed) => changeOption("matchCase", pressed)}
            >
              <span aria-hidden="true">Aa</span>
            </ToggleButton>
          </Popover>
          <Popover content={t("Match whole word")}>
            <ToggleButton
              className={styles.mode}
              variant="toolbar"
              aria-label={t("Match whole word")}
              pressed={options.wholeWord}
              onPressedChange={(pressed) => changeOption("wholeWord", pressed)}
            >
              <span className={styles.word} aria-hidden="true">
                ab
              </span>
            </ToggleButton>
          </Popover>
          <Popover content={t("Use regular expression")}>
            <ToggleButton
              className={styles.mode}
              variant="toolbar"
              aria-label={t("Use regular expression")}
              pressed={options.useRegex}
              onPressedChange={(pressed) => changeOption("useRegex", pressed)}
            >
              <span aria-hidden="true">.*</span>
            </ToggleButton>
          </Popover>
        </div>
        <Button
          className={styles.close}
          variant="toolbar"
          type="button"
          aria-label={t("Close search")}
          onClick={close}
        >
          ×
        </Button>
      </form>
      {!!searchQuery && (
        <>
          {compiled.error ? (
            <p className={styles.empty} id={errorId} role="alert">
              <Popover content={compiled.error}>
                {t("Invalid regular expression")}
              </Popover>
            </p>
          ) : (
            visible.length === 0 && (
              <p className={styles.empty}>{t("No matches")}</p>
            )
          )}
          {visible.length > 0 && (
            <ScrollArea
              className={styles.results}
              contentClassName={styles.resultContent}
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
                      {date && <DateTime value={date} />}
                      {relative && (
                        <span className={styles.relative}>- {relative}</span>
                      )}
                    </span>
                    <SearchSnippet hit={hit} />
                  </Button>
                );
              })}
            </ScrollArea>
          )}
        </>
      )}
    </section>
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
