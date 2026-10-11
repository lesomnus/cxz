import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import {
  SuggestionGrid,
  type SuggestionAnchor,
  type SuggestionGridLayout,
} from "@lesomnus/cxz-ui";
import { caretPosition } from "./editor-caret";
import { pathKind, type PathMatch } from "./composer-paths";
import styles from "./path-grid.module.css";

export function PathGrid({
  id,
  input,
  matches,
  selected,
  label,
  choose,
  caret,
  onLayoutChange,
  message,
}: {
  id: string;
  input: RefObject<HTMLTextAreaElement | null>;
  matches: PathMatch[];
  selected: number;
  label: string;
  choose: (path: string) => void;
  caret: number;
  onLayoutChange: (layout: SuggestionGridLayout) => void;
  message: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  const [anchor, setAnchor] = useState<SuggestionAnchor>({
    x: 0,
    edge: "bottom",
  });
  const items = useMemo(
    () =>
      matches.map((match) => {
        const kind = pathKind(match.entry);
        const color =
          kind === "file" && match.entry.executable ? "executable" : kind;
        const name = match.entry.name + (match.entry.directory ? "/" : "");
        const prefix = match.command.name.length - name.length;
        return {
          label: match.command.name,
          title: `${match.command.name} — ${match.command.description}${match.entry.linkTarget ? ` → ${match.entry.linkTarget}` : ""}`,
          color: `var(--path-${color}-color, var(--ui-shade-237))`,
          content: (
            <span
              className="path-entry"
              data-kind={kind}
              data-executable={match.entry.executable}
            >
              {[...name].map((char, index) => (
                <span
                  key={index}
                  data-matched={match.positions.includes(index + prefix)}
                >
                  {char}
                </span>
              ))}
            </span>
          ),
        };
      }),
    [matches],
  );
  useLayoutEffect(() => {
    const el = input.current!,
      popup = root.current!;
    const position = () => {
      const bounds = el.getBoundingClientRect(),
        style = getComputedStyle(el);
      const scale = bounds.width / el.offsetWidth || 1;
      const height = parseFloat(style.lineHeight) * scale;
      const point = caretPosition(el, caret);
      const top = bounds.top + (point.top - el.scrollTop) * scale;
      const inset = 8,
        gap = 4 * scale;
      const width = Math.min(
        el.clientWidth * scale,
        window.innerWidth - 2 * inset,
      );
      popup.style.width = `${Math.max(0, width)}px`;
      popup.style.left = `${Math.max(inset, Math.min(bounds.left, window.innerWidth - width - inset))}px`;
      popup.style.fontFamily = style.fontFamily;
      popup.style.fontSize = `${parseFloat(style.fontSize) * scale}px`;
      popup.style.lineHeight = `${height}px`;
      popup.style.setProperty(
        "--path-entry-inset",
        `${parseFloat(style.paddingLeft) * scale}px`,
      );
      const above = Math.max(0, top - inset - gap);
      const below = Math.max(
        0,
        window.innerHeight - top - height - inset - gap,
      );
      const upward = above >= popup.scrollHeight || above >= below;
      popup.style.setProperty(
        "--suggestion-grid-max-height",
        `min(var(--path-menu-height), ${upward ? above : below}px)`,
      );
      popup.style.top = `${upward ? top - gap - popup.offsetHeight : top + height + gap}px`;
      const next: SuggestionAnchor = {
        x:
          bounds.left +
          (point.left - el.scrollLeft) * scale -
          popup.getBoundingClientRect().left,
        edge: upward ? "bottom" : "top",
        distance: gap + height / 2,
      };
      setAnchor((old) =>
        old.x === next.x &&
        old.edge === next.edge &&
        old.distance === next.distance
          ? old
          : next,
      );
    };
    position();
    const resize = new ResizeObserver(position);
    resize.observe(el);
    resize.observe(popup);
    document.fonts.addEventListener("loadingdone", position);
    window.addEventListener("resize", position);
    window.addEventListener("scroll", position, true);
    return () => {
      resize.disconnect();
      document.fonts.removeEventListener("loadingdone", position);
      window.removeEventListener("resize", position);
      window.removeEventListener("scroll", position, true);
    };
  }, [input, caret, matches]);
  return createPortal(
    <div ref={root} className={styles.popup}>
      <SuggestionGrid
        id={id}
        className={styles.candidates}
        label={label}
        items={items}
        selected={selected}
        choose={(index) => choose(matches[index].command.name)}
        anchor={anchor}
        onLayoutChange={onLayoutChange}
        message={message}
      />
    </div>,
    document.body,
  );
}
