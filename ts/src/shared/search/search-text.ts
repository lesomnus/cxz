import { searchMatches } from "./search-pattern";

export function searchText(text: string, pattern?: RegExp) {
  let start = 0,
    end = 0,
    occurrences = 0;
  const snippetMatches: { start: number; end: number }[] = [];
  for (const match of searchMatches(text, pattern)) {
    if (!occurrences) {
      start = Math.max(0, match.index - 80);
      end = Math.min(text.length, start + 240);
    }
    occurrences++;
    const from = Math.max(start, match.index);
    const to = Math.min(end, match.index + match[0].length);
    if (from < to) {
      const offset = start ? 1 : 0;
      snippetMatches.push({
        start: from - start + offset,
        end: to - start + offset,
      });
    }
  }
  if (!occurrences) return;
  return {
    snippet: `${start ? "…" : ""}${text.slice(start, end)}${end < text.length ? "…" : ""}`,
    snippetMatches,
    occurrences,
  };
}
