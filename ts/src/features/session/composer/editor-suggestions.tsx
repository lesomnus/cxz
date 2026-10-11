import {
  useLayoutEffect,
  useRef,
  type CSSProperties,
  type RefObject,
} from "react";
import { Button } from "@lesomnus/cxz-ui";
import {
  COMMAND_NEIGHBORS,
  commandWindow,
  type CommandMatch,
} from "./composer-commands";

// Measure the native caret's wrapped line without altering the input or its
// selection. The offscreen mirror uses the textarea's actual font and width.
function caretTop(input: HTMLTextAreaElement, offset: number) {
  const style = getComputedStyle(input);
  const mirror = document.createElement("div");
  Object.assign(mirror.style, {
    position: "fixed",
    visibility: "hidden",
    left: "-10000px",
    top: "0",
    width: `${input.clientWidth}px`,
    boxSizing: "border-box",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    font: style.font,
    lineHeight: style.lineHeight,
    letterSpacing: style.letterSpacing,
    tabSize: style.tabSize,
    padding: style.padding,
    textIndent: style.textIndent,
  });
  mirror.textContent = input.value.slice(0, offset);
  const marker = document.createElement("span");
  marker.textContent = input.value[offset] || ".";
  mirror.append(marker);
  document.body.append(mirror);
  const top =
    marker.getBoundingClientRect().top - mirror.getBoundingClientRect().top;
  mirror.remove();
  return top;
}

export function EditorSuggestions({
  id,
  input,
  matches,
  selected,
  label,
  choose,
  caret,
  message,
}: {
  id: string;
  input: RefObject<HTMLTextAreaElement | null>;
  matches: CommandMatch[];
  selected: number;
  label: string;
  choose: (name: string) => void;
  caret?: number;
  message?: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  const rows = commandWindow(matches, selected);
  useLayoutEffect(() => {
    const el = input.current!,
      overlay = root.current!;
    const measure = () => {
      const style = getComputedStyle(el),
        height = parseFloat(style.lineHeight);
      const inset = parseFloat(getComputedStyle(overlay).paddingTop);
      const top =
        caret === undefined
          ? parseFloat(style.paddingTop)
          : caretTop(el, caret);
      overlay.style.top = `${el.offsetTop + top - el.scrollTop - COMMAND_NEIGHBORS * height - inset}px`;
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
  }, [input, caret, matches]);
  return (
    <div
      className="command-suggestions"
      data-path={caret !== undefined}
      ref={root}
      id={id}
      role="listbox"
      aria-label={label}
      style={
        {
          "--command-name-width": `${Math.max(0, ...rows.map((match) => match?.command.name.length ?? 0)) + 1}ch`,
        } as CSSProperties
      }
    >
      {rows.length ? (
        rows.map((match, row) =>
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
              onClick={() => choose(match.command.name)}
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
        )
      ) : (
        <div className="command-suggestion" role="status">
          {message}
        </div>
      )}
    </div>
  );
}
