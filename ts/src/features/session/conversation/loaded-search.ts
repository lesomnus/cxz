import type { SessionEvent } from "#gen/cxz/session_pb";
import { conversationMarkdown } from "#src/shared/content/markdown-parser.ts";
import DOMPurify from "dompurify";

export type LoadedSearchHit = Pick<SessionEvent, "seq" | "timeMs" | "kind"> & {
  snippet: string;
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

export function literalSearchPattern(query: string) {
  return new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "giu");
}

export function searchLoadedMessages(
  events: SessionEvent[],
  query: string,
): LoadedSearchHit[] {
  if (!query) return [];
  const hits: LoadedSearchHit[] = [];
  for (const event of events) {
    if (event.kind !== "input" && event.kind !== "assistant") continue;
    const text = bodyText(event);
    let first = -1,
      occurrences = 0;
    for (const match of text.matchAll(literalSearchPattern(query))) {
      if (first < 0) first = match.index;
      occurrences++;
    }
    if (!occurrences) continue;
    const start = Math.max(0, first - 80);
    hits.push({
      seq: event.seq,
      timeMs: event.timeMs,
      kind: event.kind,
      snippet: `${start ? "…" : ""}${text.slice(start, start + 240)}${start + 240 < text.length ? "…" : ""}`,
      occurrences,
    });
  }
  return hits.sort((a, b) => (a.seq > b.seq ? -1 : a.seq < b.seq ? 1 : 0));
}
