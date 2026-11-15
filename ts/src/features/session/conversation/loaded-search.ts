import type { SessionEvent } from "#gen/cxz/session_pb";
import { conversationMarkdown } from "#src/shared/content/markdown-parser.ts";
import DOMPurify from "dompurify";
import { searchText } from "#src/shared/search/search-text.ts";
export { searchText } from "#src/shared/search/search-text.ts";

export type LoadedSearchHit = Pick<SessionEvent, "seq" | "timeMs" | "kind"> & {
  snippet: string;
  snippetMatches: { start: number; end: number }[];
  occurrences: number;
};

// Cache strings, never detached DOM. Weak keys let evicted messages disappear.
const textCache = new WeakMap<SessionEvent, { source: string; text: string }>();
function bodyText(event: SessionEvent) {
  const saved = textCache.get(event);
  if (saved?.source === event.text) return saved.text;
  const html = DOMPurify.sanitize(
    conversationMarkdown.parse(event.text, { async: false }),
    {
      FORBID_TAGS: ["img", "style", "input", "form"],
      FORBID_ATTR: ["style"],
    },
  );
  const doc = new DOMParser().parseFromString(html, "text/html");
  for (const control of doc.body.querySelectorAll(
    'button, select, [aria-hidden="true"]',
  ))
    control.remove();
  // Preserve visible block/line breaks rather than joining adjacent paragraphs.
  for (const block of doc.body.querySelectorAll(
    "p, li, pre, td, th, h1, h2, h3, h4, h5, h6, blockquote, br",
  ))
    block.append(doc.createTextNode("\n"));
  const text = doc.body.textContent ?? "";
  textCache.set(event, { source: event.text, text });
  return text;
}

export function searchLoadedMessages(
  events: SessionEvent[],
  pattern?: RegExp,
): LoadedSearchHit[] {
  if (!pattern) return [];
  const hits: LoadedSearchHit[] = [];
  for (const event of events) {
    if (event.kind !== "input" && event.kind !== "assistant") continue;
    const result = searchText(bodyText(event), pattern);
    if (!result) continue;
    hits.push({
      seq: event.seq,
      timeMs: event.timeMs,
      kind: event.kind,
      ...result,
    });
  }
  return hits.sort((a, b) => (a.seq > b.seq ? -1 : a.seq < b.seq ? 1 : 0));
}
