export type ElidedPath = {
  prefix: string;
  leaf: string;
  middle: boolean;
  end: boolean;
  separator: string;
};

// Keep the longest leading directory prefix that fits beside the whole leaf.
// The first two directories take priority over an oversized leaf.
export function elidePath(
  value: string,
  width: number,
  measure: (text: string) => number,
  markWidth: number,
): ElidedPath {
  const parts = value.match(/^(.*[/\\])([^/\\]+[/\\]*)$/);
  const parent = parts?.[1] ?? "";
  const leaf = parts?.[2] ?? value;
  const separator = parent.slice(-1);
  const full = { prefix: parent, leaf, middle: false, end: false, separator };
  if (measure(value) <= width) return full;

  const root = parent.match(/^(?:[a-zA-Z]:)?[/\\]+/)?.[0] ?? "";
  const directories = parent.slice(root.length).match(/[^/\\]+[/\\]+/g) ?? [];
  const minimum = Math.min(2, directories.length);
  const prefixAt = (count: number) =>
    root + directories.slice(0, count).join("");
  const leafWidth = measure(leaf);
  const middleWidth = markWidth + measure(separator);
  for (let count = directories.length - 1; count >= minimum; count--) {
    const prefix = prefixAt(count);
    if (measure(prefix) + middleWidth + leafWidth <= width)
      return { prefix, leaf, middle: true, end: false, separator };
  }

  const prefix = prefixAt(minimum);
  const middle = directories.length > minimum;
  const remaining = width - measure(prefix) - (middle ? middleWidth : 0);
  if (leafWidth <= remaining)
    return { prefix, leaf, middle, end: false, separator };

  // Grapheme boundaries avoid splitting a combined character or emoji.
  const characters = Array.from(
    new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(leaf),
    (part) => part.segment,
  );
  const budget = Math.max(0, remaining - markWidth);
  let low = 0,
    high = characters.length;
  while (low < high) {
    const next = Math.ceil((low + high) / 2);
    if (measure(characters.slice(0, next).join("")) <= budget) low = next;
    else high = next - 1;
  }
  return {
    prefix,
    leaf: characters.slice(0, low).join(""),
    middle,
    end: true,
    separator,
  };
}
