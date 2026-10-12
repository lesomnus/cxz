import {
  useEffect,
  useId,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import type { Session } from "#gen/cxz/session_pb";
import { create } from "@bufbuild/protobuf";
import { ProjectPathEntrySchema } from "#gen/cxz/project_svc_pb";
import {
  moveRankedGridSelection,
  type SuggestionGridLayout,
} from "@lesomnus/cxz-ui";
import { PathGrid } from "./path-grid";
import { mentionQuery, mentionMatches, redactQuery } from "./composer-mentions";
import { matchCommands } from "./composer-commands";

export type MentionContext = {
  current: Session;
  sessions: Session[];
  redact?: (start: number, end: number) => void;
};
export function useMentionSuggestions({
  value,
  input,
  context,
  composing,
  accept,
}: {
  value: string;
  input: RefObject<HTMLTextAreaElement | null>;
  context?: MentionContext;
  composing: boolean;
  accept: (start: number, end: number, text: string) => void;
}) {
  const id = useId();
  const [selection, setSelection] = useState({
    focused: false,
    start: 0,
    end: 0,
  });
  const [choice, setChoice] = useState({
    signature: "",
    index: 0,
    dismissed: false,
  });
  const [layout, setLayout] = useState<SuggestionGridLayout>({
    columns: 1,
    order: [],
  });
  const token =
    context && selection.focused && !composing
      ? (mentionQuery(value, selection.start, selection.end) ??
        (context.redact
          ? redactQuery(value, selection.start, selection.end)
          : undefined))
      : undefined;
  const signature = token ? `${token.start}:${token.query}` : "";
  const matches =
    token && context
      ? token.query.startsWith("/")
        ? matchCommands(
            [
              {
                name: "/@redact",
                description: "Insert a secret file reference",
              },
            ],
            token.query,
          )
        : mentionMatches(context.sessions, context.current, token.query)
      : [];
  const selected =
    choice.signature === signature
      ? Math.min(choice.index, Math.max(0, matches.length - 1))
      : 0;
  const open =
    !!token &&
    matches.length > 0 &&
    !(choice.signature === signature && choice.dismissed);
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
  function commit(name: string) {
    if (!token) return;
    setChoice({ signature, index: selected, dismissed: true });
    if (name === "/@redact") {
      context?.redact?.(token.start, token.end);
      return;
    }
    accept(
      token.start,
      token.end,
      name + (token.end === value.length ? " " : ""),
    );
  }
  function key(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      !open ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey ||
      event.shiftKey ||
      event.nativeEvent.isComposing
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
    event.preventDefault();
    event.stopPropagation();
    if (event.key === "Escape")
      setChoice({ signature, index: selected, dismissed: true });
    else if (event.key === "Tab" || event.key === "Enter")
      commit(matches[selected].command.name);
    else
      setChoice({
        signature,
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
      "aria-controls": open ? id : undefined,
      "aria-activedescendant": open ? `${id}-${selected}` : undefined,
    },
    overlay: open && (
      <PathGrid
        id={id}
        input={input}
        caret={selection.start}
        label="Session mentions"
        selected={selected}
        matches={matches.map((match) => ({
          ...match,
          entry: create(ProjectPathEntrySchema, { name: match.command.name }),
        }))}
        choose={commit}
        onLayoutChange={setLayout}
        message="Sessions in this project"
      />
    ),
  };
}
