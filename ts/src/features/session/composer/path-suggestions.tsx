import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import type { ProjectPathEntry } from "#gen/cxz/project_svc_pb";
import { ConnectError } from "@connectrpc/connect";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  containerPathQuery,
  matchPaths,
  type LookupPaths,
} from "./composer-paths";
import {
  moveRankedGridSelection,
  type SuggestionGridLayout,
} from "@lesomnus/nox";
import { PathGrid } from "./path-grid";

export function usePathSuggestions({
  value,
  input,
  composing,
  lookup,
  workspace = "",
  accept,
}: {
  value: string;
  input: RefObject<HTMLTextAreaElement | null>;
  composing: boolean;
  lookup?: LookupPaths;
  workspace?: string;
  accept: (start: number, end: number, path: string) => void;
}) {
  useLocale();
  const id = useId();
  const [layout, setLayout] = useState<SuggestionGridLayout>({
    columns: 1,
    order: [],
  });
  const [selection, setSelection] = useState({
    focused: false,
    start: 0,
    end: 0,
  });
  const [choice, setChoice] = useState({
    text: "",
    index: 0,
    dismissed: false,
  });
  const [listing, setListing] = useState<{
    lookup: LookupPaths;
    parent: string;
    entries: ProjectPathEntry[];
    error?: string;
  }>();
  const cache = useRef(
    new Map<string, { entries: ProjectPathEntry[]; time: number }>(),
  );
  const token =
    lookup && selection.focused && !composing
      ? containerPathQuery(value, selection.start, selection.end, workspace)
      : undefined;
  const signature = token ? `${token.start}:${token.text}` : "";
  const parent = token?.parent;
  const current =
    listing?.lookup === lookup && listing?.parent === parent
      ? listing
      : undefined;
  const matches = token && current ? matchPaths(current.entries, token) : [];
  const selected =
    choice.text === signature
      ? Math.min(choice.index, Math.max(0, matches.length - 1))
      : 0;
  const open = !!token && !(choice.text === signature && choice.dismissed);
  function updateSelection() {
    const el = input.current;
    if (!el) return;
    const next = {
      focused: document.activeElement === el,
      start: el.selectionStart,
      end: el.selectionEnd,
    };
    setSelection((old) =>
      old.focused === next.focused &&
      old.start === next.start &&
      old.end === next.end
        ? old
        : next,
    );
  }
  useEffect(() => {
    const el = input.current!;
    el.addEventListener("select", updateSelection);
    updateSelection();
    return () => el.removeEventListener("select", updateSelection);
  }, [input]);
  useEffect(() => {
    cache.current.clear();
  }, [lookup]);
  useEffect(() => {
    if (!lookup || !parent) return;
    const cached = cache.current.get(parent);
    if (cached && Date.now() - cached.time < 5000) {
      setListing({ lookup, parent, entries: cached.entries });
      return;
    }
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void lookup(parent, controller.signal)
        .then((entries) => {
          if (controller.signal.aborted) return;
          if (cache.current.size >= 32)
            cache.current.delete(cache.current.keys().next().value!);
          cache.current.set(parent, { entries, time: Date.now() });
          setListing({ lookup, parent, entries });
        })
        .catch((error: unknown) => {
          if (!controller.signal.aborted)
            setListing({
              lookup,
              parent,
              entries: [],
              error: `${t("Directory unavailable")}: ${parent} — ${ConnectError.from(error).message}`,
            });
        });
    }, 75);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [lookup, parent]);
  function commit(path: string) {
    if (!token) return;
    setChoice({
      text: `${token.start}:${path}`,
      index: 0,
      dismissed: !path.endsWith("/"),
    });
    accept(token.start, token.end, path);
  }
  function key(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      !open ||
      event.nativeEvent.isComposing ||
      event.ctrlKey ||
      event.altKey ||
      event.metaKey ||
      event.shiftKey
    )
      return false;
    if (
      ![
        "ArrowUp",
        "ArrowDown",
        "ArrowLeft",
        "ArrowRight",
        "Tab",
        "Enter",
        "Escape",
      ].includes(event.key)
    )
      return false;
    if (!matches.length && event.key !== "Escape") return false;
    event.preventDefault();
    event.stopPropagation();
    if (event.key === "Escape")
      setChoice({ text: signature, index: selected, dismissed: true });
    else if (event.key === "Enter" || event.key === "Tab")
      commit(matches[selected].command.name);
    else
      setChoice({
        text: signature,
        index: moveRankedGridSelection(
          selected,
          layout.order,
          layout.columns,
          event.key,
        ),
        dismissed: false,
      });
    return true;
  }
  return {
    open,
    key,
    updateSelection,
    inputProps: {
      "aria-autocomplete": lookup ? ("list" as const) : undefined,
      "aria-haspopup": open ? ("grid" as const) : undefined,
      "aria-controls": open ? id : undefined,
      "aria-activedescendant":
        open && matches.length ? `${id}-${selected}` : undefined,
    },
    overlay: open && (
      <PathGrid
        id={id}
        input={input}
        matches={matches}
        selected={selected}
        label={t("Path suggestions")}
        choose={commit}
        caret={selection.start}
        onLayoutChange={setLayout}
        message={
          !current
            ? t("Loading paths…")
            : (current.error ?? t("No matching paths"))
        }
      />
    ),
  };
}
