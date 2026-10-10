import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import { Button } from "@lesomnus/cxz-ui";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  COMMAND_NEIGHBORS,
  commandQuery,
  commandWindow,
  matchCommands,
  type ComposerCommand,
} from "./composer-commands";

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
  const root = useRef<HTMLDivElement>(null);
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

  useLayoutEffect(() => {
    if (!open) return;
    const el = input.current!,
      overlay = root.current!;
    const measure = () => {
      const style = getComputedStyle(el);
      const height = parseFloat(style.lineHeight);
      const inset = parseFloat(getComputedStyle(overlay).paddingTop);
      overlay.style.top = `${el.offsetTop + parseFloat(style.paddingTop) - el.scrollTop - COMMAND_NEIGHBORS * height - inset}px`;
      overlay.style.left = `${el.offsetLeft}px`;
      overlay.style.width = `${el.clientWidth}px`;
      overlay.style.setProperty("--command-line-height", `${height}px`);
      overlay.style.setProperty("--command-inset", style.paddingLeft);
    };
    measure();
    const resize = new ResizeObserver(measure);
    resize.observe(el);
    resize.observe(overlay);
    el.addEventListener("scroll", measure);
    return () => {
      resize.disconnect();
      el.removeEventListener("scroll", measure);
    };
  }, [open, input, value]);

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
  const rows = open ? commandWindow(matches, selected) : [];
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
      <div
        className="command-suggestions"
        ref={root}
        id={id}
        role="listbox"
        aria-label={t("Command suggestions")}
        style={
          {
            "--command-name-width": `${Math.max(...rows.map((match) => match?.command.name.length ?? 0)) + 1}ch`,
          } as React.CSSProperties
        }
      >
        {rows.map((match, row) =>
          match ? (
            <Button
              key={row}
              id={`${id}-${row}`}
              type="button"
              role="option"
              tabIndex={-1}
              aria-selected={row === COMMAND_NEIGHBORS}
              className="command-suggestion"
              data-current={row === COMMAND_NEIGHBORS}
              title={`${match.command.name} — ${match.command.description}`}
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => commit(match.command.name)}
            >
              <span className="command-name">
                {[...match.command.name].map((char, index) => (
                  <span
                    key={index}
                    data-matched={match.positions.includes(index)}
                  >
                    {char}
                  </span>
                ))}
              </span>
              <span className="command-description">
                {match.command.description}
              </span>
            </Button>
          ) : (
            <div
              key={row}
              className="command-suggestion command-empty"
              aria-hidden="true"
            />
          ),
        )}
      </div>
    ),
  };
}
