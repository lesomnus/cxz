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

export function EditorSuggestions({
  id,
  input,
  matches,
  selected,
  label,
  choose,
}: {
  id: string;
  input: RefObject<HTMLTextAreaElement | null>;
  matches: CommandMatch[];
  selected: number;
  label: string;
  choose: (name: string) => void;
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
      const top = parseFloat(style.paddingTop);
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
  }, [input, matches]);
  return (
    <div
      className="command-suggestions"
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
        <div className="command-suggestion" role="status"></div>
      )}
    </div>
  );
}
