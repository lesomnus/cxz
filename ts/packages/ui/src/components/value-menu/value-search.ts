export type ValueMatch = { start: number; end: number };
const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });

// Return original-text ranges so normalization never splits a displayed grapheme.
// Prefer a contiguous hit, then fall back to ordered abbreviation matching.
export function matchValueQuery(
  text: string,
  query: string,
): ValueMatch[] | null {
  const needle = query.normalize("NFKC").toLowerCase().replace(/\s/g, "");
  if (!needle) return [];
  let haystack = "";
  const offsets: ValueMatch[] = [];
  for (const { segment, index } of graphemes.segment(text)) {
    const normalized = segment.normalize("NFKC").toLowerCase();
    haystack += normalized;
    for (let i = 0; i < normalized.length; i++)
      offsets.push({ start: index, end: index + segment.length });
  }
  const positions: number[] = [];
  const contiguous = haystack.indexOf(needle);
  if (contiguous >= 0) {
    for (let i = 0; i < needle.length; i++) positions.push(contiguous + i);
  } else {
    let from = 0;
    for (const char of needle) {
      const at = haystack.indexOf(char, from);
      if (at < 0) return null;
      for (let i = 0; i < char.length; i++) positions.push(at + i);
      from = at + char.length;
    }
  }
  const ranges: ValueMatch[] = [];
  for (const position of positions) {
    const next = offsets[position];
    const last = ranges.at(-1);
    if (last && next.start <= last.end) last.end = Math.max(last.end, next.end);
    else ranges.push({ ...next });
  }
  return ranges;
}

export function matchesValueQuery(text: string, query: string): boolean {
  return matchValueQuery(text, query) !== null;
}
