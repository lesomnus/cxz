import { useEffect, useRef, type RefObject } from "react";
import { literalSearchPattern } from "./loaded-search";

export type SearchLocation = {
  seq: string;
  occurrence: number;
  inset: number;
  token: number;
};

// Match rendered text, including Markdown split across inline syntax spans.
// Keep block boundaries separate and leave React's DOM and row heights intact.
export function textMatches(root: HTMLElement, query: string): Range[] {
  if (!query) return [];
  const nodes: { node: Text; start: number; end: number }[] = [];
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let text = "",
    previous: Element | null = null;
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (node.parentElement?.closest("button, select, [aria-hidden='true']"))
      continue;
    const block =
      node.parentElement?.closest(
        "p, li, pre, td, th, h1, h2, h3, h4, h5, h6, blockquote",
      ) ?? root;
    if (previous && block !== previous) text += "\n";
    previous = block;
    const start = text.length;
    text += node.textContent ?? "";
    nodes.push({ node: node as Text, start, end: text.length });
  }
  const pattern = literalSearchPattern(query);
  const ranges: Range[] = [];
  for (const match of text.matchAll(pattern)) {
    const start = match.index,
      end = start + match[0].length;
    const first = nodes.find((part) => part.start <= start && part.end > start);
    const last = nodes.find((part) => part.start < end && part.end >= end);
    if (!first || !last) continue;
    const range = new Range();
    range.setStart(first.node, start - first.start);
    range.setEnd(last.node, end - last.start);
    ranges.push(range);
  }
  return ranges;
}

export function conversationMatches(area: HTMLElement, query: string) {
  const found = new Map<string, Range[]>();
  for (const row of area.querySelectorAll<HTMLElement>(
    ".transcript .virtual-messages [data-row]",
  )) {
    const ranges = [
      ...row.querySelectorAll<HTMLElement>(
        "article.input .message-body, article.response > .markdown",
      ),
    ].flatMap((body) => textMatches(body, query));
    if (ranges.length) found.set(row.dataset.row!, ranges);
  }
  return found;
}

export function useSearchHighlights({
  area,
  query,
  location,
  located,
}: {
  area: RefObject<HTMLElement | null>;
  query: string;
  location: SearchLocation | undefined;
  located: (occurrence: number) => void;
}) {
  const matches = useRef(new Map<string, Range[]>());
  const landed = useRef<number | undefined>(undefined);
  const callback = useRef(located);
  callback.current = located;
  useEffect(() => {
    const root = area.current;
    if (!root || !query) {
      matches.current.clear();
      return;
    }
    let frame = 0;
    const update = () => {
      matches.current = conversationMatches(root, query);
      const all = [...matches.current.values()].flat();
      const ranges = location && matches.current.get(location.seq);
      const index = ranges?.length
        ? location!.occurrence < 0
          ? ranges.length - 1
          : Math.min(location!.occurrence, ranges.length - 1)
        : -1;
      const range = index >= 0 ? ranges![index] : undefined;
      if (typeof Highlight !== "undefined" && CSS.highlights) {
        CSS.highlights.set("conversation-search", new Highlight(...all));
        const current = new Highlight(...(range ? [range] : []));
        current.priority = 1;
        CSS.highlights.set("conversation-search-current", current);
      }
      if (!location || !range || landed.current === location.token) return;
      const pane = root.querySelector<HTMLElement>(".transcript");
      const rect = range.getClientRects()[0];
      if (!pane || !rect) return;
      landed.current = location.token;
      pane.dispatchEvent(new Event("scroll-jump"));
      pane.scrollTop +=
        rect.top - pane.getBoundingClientRect().top - location.inset;
      pane.dispatchEvent(
        new CustomEvent("reading-anchor", { detail: location.seq }),
      );
      callback.current(index);
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    // Only mounted messages are scanned. Virtualization and streaming replace
    // text nodes; a batched observer refreshes ranges without retaining old DOM.
    const observer = new MutationObserver(schedule);
    const transcript = root.querySelector(".transcript") ?? root;
    observer.observe(transcript, {
      subtree: true,
      childList: true,
      characterData: true,
    });
    schedule();
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
      matches.current.clear();
      if (typeof CSS !== "undefined" && CSS.highlights) {
        CSS.highlights.delete("conversation-search");
        CSS.highlights.delete("conversation-search-current");
      }
    };
  }, [area, query, location]);
  return matches;
}
