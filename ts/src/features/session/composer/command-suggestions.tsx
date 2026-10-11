import {
  useEffect,
  useId,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  COMMAND_NEIGHBORS,
  commandQuery,
  matchCommands,
  type ComposerCommand,
} from "./composer-commands";

import { EditorSuggestions } from "./editor-suggestions";

export function useCommandSuggestions({
  value,
  commands,
  input,
  composing,
  accept,
}: {
  value: string;
  commands?: readonly ComposerCommand[];
  input: RefObject<HTMLTextAreaElement | null>;
  composing: boolean;
  accept: (name: string, end: number) => void;
}) {
  useLocale();
  const id = useId();
  const [selection, setSelection] = useState({
    focused: false,
    start: 0,
    end: 0,
  });
  const [choice, setChoice] = useState({
    query: "",
    index: 0,
    dismissed: false,
  });
  const query =
    selection.focused && !composing && commands?.length
      ? commandQuery(value, selection.start, selection.end)
      : undefined;
  const matches = query === undefined ? [] : matchCommands(commands!, query);
  const selected =
    choice.query === query ? Math.min(choice.index, matches.length - 1) : 0;
  const open =
    matches.length > 0 && !(choice.query === query && choice.dismissed);
  const suggestion = matches[selected];

  function updateSelection() {
    const el = input.current!;
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
    // Native select also covers programmatic cursor changes. Avoid native input
    // listeners: they can flush a controlled textarea before React's onChange.
    el.addEventListener("select", updateSelection);
    updateSelection();
    return () => el.removeEventListener("select", updateSelection);
  }, [input]);
  const firstLine = value.split("\n", 1)[0];
  useEffect(() => {
    setChoice((old) =>
      old.query && old.query !== firstLine
        ? { query: "", index: 0, dismissed: false }
        : old,
    );
  }, [firstLine]);

  function commit(name: string) {
    setChoice({ query: name, index: 0, dismissed: true });
    accept(name, query!.length);
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
    if (!["ArrowUp", "ArrowDown", "ArrowRight", "Escape"].includes(event.key))
      return false;
    event.preventDefault();
    event.stopPropagation();
    if (event.key === "Escape")
      setChoice({ query: query!, index: selected, dismissed: true });
    else if (event.key === "ArrowRight") commit(suggestion.command.name);
    else
      setChoice({
        query: query!,
        index:
          (selected + (event.key === "ArrowDown" ? 1 : -1) + matches.length) %
          matches.length,
        dismissed: false,
      });
    return true;
  }
  return {
    open,
    key,
    updateSelection,
    inputProps: {
      "aria-autocomplete": commands?.length ? ("both" as const) : undefined,
      "aria-controls": open ? id : undefined,
      "aria-activedescendant": open ? `${id}-${COMMAND_NEIGHBORS}` : undefined,
    },
    overlay: open && (
      <EditorSuggestions
        id={id}
        input={input}
        matches={matches}
        selected={selected}
        label={t("Command suggestions")}
        choose={commit}
      />
    ),
  };
}
