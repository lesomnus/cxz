import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import {
  FindBar,
  FindBarClose,
  FindBarControls,
  FindBarCounter,
  FindBarIcon,
  FindBarInput,
  FindBarMessage,
  FindBarOptions,
  FindBarResult,
  FindBarResults,
  Popover,
} from "@lesomnus/nox";
import {
  compileSearchPattern,
  defaultSearchOptions,
  type SearchOptions,
} from "#src/shared/search/search-pattern.ts";
import { searchText } from "#src/shared/search/search-text.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { DateTime, RelativeTime } from "./date-time";
import type { DateTimeValue } from "./date-time-format";
import type { ResourceSearchController } from "./use-resource-search";
import styles from "./resource-search.module.css";

export type ResourceSearchItem = {
  id: string;
  title: string;
  text: string;
  date?: DateTimeValue;
  icon?: ReactNode;
};

export function ResourceSearch({
  search,
  items,
  label,
  loading = false,
  error = "",
  select,
}: {
  search: ResourceSearchController;
  items: ResourceSearchItem[];
  label: string;
  loading?: boolean;
  error?: string;
  select: (id: string) => void;
}) {
  useLocale();
  const [query, setQuery] = useState("");
  const [options, setOptions] = useState(defaultSearchOptions);
  const [selected, setSelected] = useState<string>();
  const errorId = useId();
  const results = useRef<HTMLDivElement>(null);
  const searchQuery = options.useRegex ? query : query.trim();
  const compiled = useMemo(
    () => compileSearchPattern(searchQuery, options),
    [searchQuery, options],
  );
  const hits = useMemo(() => {
    if (!search.open || !compiled.pattern) return [];
    return items.flatMap((item) => {
      const match = searchText(item.text, compiled.pattern);
      return match ? [{ item, ...match }] : [];
    });
  }, [items, search.open, compiled.pattern]);
  const index = Math.max(
    0,
    hits.findIndex((hit) => hit.item.id === selected),
  );
  useEffect(() => {
    if (search.open)
      results.current
        ?.querySelector<HTMLElement>(`[data-search-index="${index}"]`)
        ?.scrollIntoView({ block: "nearest" });
  }, [index, search.open]);
  function choose(id: string) {
    setSelected(id);
    select(id);
    requestAnimationFrame(() =>
      search.input.current?.focus({ preventScroll: true }),
    );
  }
  if (!search.open) return null;
  return (
    <FindBar
      ref={search.overlay}
      motion
      role="search"
      aria-label={label}
      className={styles.overlay}
    >
      <FindBarControls>
        <FindBarIcon />
        <FindBarInput
          ref={search.input}
          type="search"
          value={query}
          aria-label={label}
          placeholder={label}
          autoComplete="off"
          aria-invalid={!!compiled.error || undefined}
          aria-describedby={compiled.error ? errorId : undefined}
          onChange={(event) => {
            setQuery(event.target.value);
            setSelected(undefined);
          }}
          onKeyDown={(event) => {
            if (event.nativeEvent.isComposing || !hits.length) return;
            if (event.key === "Enter") {
              event.preventDefault();
              choose(hits[index]!.item.id);
            } else if (event.key === "ArrowUp" || event.key === "ArrowDown") {
              event.preventDefault();
              setSelected(
                hits[
                  (index + (event.key === "ArrowDown" ? 1 : -1) + hits.length) %
                    hits.length
                ]!.item.id,
              );
            }
          }}
        />
        <FindBarCounter
          current={hits.length ? index + 1 : 0}
          count={hits.length}
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
          onChange={(id, pressed) => {
            setOptions((current) => ({
              ...current,
              [id as keyof SearchOptions]: pressed,
            }));
            setSelected(undefined);
            search.input.current?.focus({ preventScroll: true });
          }}
        />
        <FindBarClose onClick={search.close} />
      </FindBarControls>
      {error && <FindBarMessage role="alert">{error}</FindBarMessage>}
      {loading && (
        <FindBarMessage role="status">{t("Loading…")}</FindBarMessage>
      )}
      {!!searchQuery && (
        <>
          {compiled.error ? (
            <FindBarMessage id={errorId} role="alert">
              <Popover content={compiled.error}>
                {t("Invalid regular expression")}
              </Popover>
            </FindBarMessage>
          ) : (
            !loading &&
            !error &&
            !hits.length && <FindBarMessage>{t("No matches")}</FindBarMessage>
          )}
          {!!hits.length && (
            <FindBarResults
              viewportRef={results}
              scrollbarOffset={-5}
              scrollbarLabel={t("Search results")}
              style={{ "--search-result-count": hits.length } as CSSProperties}
            >
              {hits.map((hit, row) => (
                <FindBarResult
                  key={hit.item.id}
                  type="button"
                  data-search-index={row}
                  aria-pressed={row === index}
                  onClick={() => choose(hit.item.id)}
                >
                  <span className={styles.meta}>
                    {hit.item.date !== undefined && (
                      <>
                        <DateTime value={hit.item.date} />
                        <span>
                          - <RelativeTime value={hit.item.date} />
                        </span>
                      </>
                    )}
                  </span>
                  <span className={styles.title}>
                    {hit.item.icon}
                    <span>{hit.item.title}</span>
                  </span>
                  <span className={styles.snippet}>
                    {highlightSnippet(hit)}
                  </span>
                </FindBarResult>
              ))}
            </FindBarResults>
          )}
        </>
      )}
    </FindBar>
  );
}

function highlightSnippet(hit: NonNullable<ReturnType<typeof searchText>>) {
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
  return content;
}
