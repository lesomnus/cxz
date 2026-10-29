// Figure spaces reserve an indivisible numeric-width area in native text.
export const PASTE_CHIP_PREFIX = "\u2007".repeat(5);

const characters = new Intl.Segmenter(undefined, { granularity: "grapheme" });
export function pasteExcerpt(body: string, limit = 10) {
  let result = "",
    count = 0;
  for (const { segment } of characters.segment(body)) {
    if (count++ === limit) return result + "…";
    result += segment.replace(/\r\n|[\r\n\u2028\u2029]/g, " ");
  }
  return result;
}
