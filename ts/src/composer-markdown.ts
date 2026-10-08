import { codeBlocks, type CodeBlock } from "./composer-code";

// One native replacement keeps list continuation in the browser's undo history.
export function listNewlineEdit(
  value: string,
  start: number,
  end: number,
  blocks: CodeBlock[] = codeBlocks(value),
) {
  if (blocks.some((block) => start >= block.start && start <= block.end))
    return;
  const from = start > 0 ? value.lastIndexOf("\n", start - 1) + 1 : 0;
  const newline = value.indexOf("\n", start);
  const to = newline < 0 ? value.length : newline;
  const line = value.slice(from, to);
  const item = /^([\t ]*)([-+*])[\t ]+(?:\[[ xX]\][\t ]+)?/.exec(line);
  if (!item || start < from + item[0].length) return;
  // Enter on an empty item exits the list without leaving a stray marker.
  if (!line.slice(item[0].length).trim())
    return { from, to, text: item[1], caret: from + item[1].length };
  const checkbox = /\[[ xX]\]/.test(item[0]) ? "[ ] " : "";
  const text = "\n" + item[1] + item[2] + " " + checkbox;
  return { from: start, to: end, text, caret: start + text.length };
}

// Pair equal-length backtick runs without changing the visible Markdown bytes.
// Longer delimiters can contain literal backticks. Unmatched/escaped openers
// remain ordinary text; backslashes inside code do not escape its closing fence.
export function inlineCodeRanges(line: string) {
  const runs = [...line.matchAll(/`+/g)];
  const next = new Map<number, number>();
  const closing: (number | undefined)[] = [];
  for (let i = runs.length - 1; i >= 0; i--) {
    const length = runs[i][0].length;
    closing[i] = next.get(length);
    next.set(length, i);
  }
  const ranges: { start: number; end: number }[] = [];
  for (let i = 0; i < runs.length; i++) {
    const run = runs[i];
    let slashes = 0;
    for (let j = run.index - 1; j >= 0 && line[j] === "\\"; j--) slashes++;
    if (slashes % 2 || closing[i] === undefined) continue;
    const close = closing[i]!;
    ranges.push({
      start: run.index,
      end: runs[close].index + runs[close][0].length,
    });
    i = close;
  }
  return ranges;
}
