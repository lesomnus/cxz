const lines = (text: string) => text.match(/[^\n]*\n|[^\n]+$/g) ?? [];
const invalid = () =>
  new Error("The comparison does not match the selected document.");

/** Restore the previous body, verifying every hunk against the cached selected body. */
export function reverseMemoryDiff(current: string, patch: string): string {
  if (!patch) return current;
  const source = lines(current),
    changes = lines(patch),
    restored: string[] = [];
  let cursor = 0,
    hunks = 0,
    index = 0;
  while (index < changes.length && !changes[index].startsWith("@@")) index++;
  while (index < changes.length) {
    const header = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(
      changes[index++],
    );
    if (!header) throw invalid();
    const oldStart = Number(header[1]),
      oldCount = Number(header[2] ?? 1);
    const newStart = Number(header[3]),
      newCount = Number(header[4] ?? 1);
    const before: string[] = [],
      after: string[] = [];
    let previous = "";
    while (index < changes.length && !changes[index].startsWith("@@")) {
      const line = changes[index++],
        kind = line[0];
      if (line.startsWith("\\ No newline at end of file")) {
        if (!previous) throw invalid();
        for (const side of previous === " "
          ? [before, after]
          : previous === "-"
            ? [before]
            : [after]) {
          if (!side.length || !side.at(-1)!.endsWith("\n")) throw invalid();
          side[side.length - 1] = side.at(-1)!.slice(0, -1);
        }
        previous = "";
        continue;
      }
      if (kind !== " " && kind !== "+" && kind !== "-") throw invalid();
      if (kind !== "+") before.push(line.slice(1));
      if (kind !== "-") after.push(line.slice(1));
      previous = kind;
    }
    const start = newCount === 0 ? newStart : newStart - 1;
    if (
      ![oldStart, oldCount, newStart, newCount].every(Number.isSafeInteger) ||
      start < cursor ||
      start + newCount > source.length ||
      before.length !== oldCount ||
      after.length !== newCount ||
      source.slice(start, start + newCount).join("") !== after.join("")
    )
      throw invalid();
    for (let line = cursor; line < start; line++) restored.push(source[line]);
    if (restored.length !== (oldCount === 0 ? oldStart : oldStart - 1))
      throw invalid();
    for (const line of before) restored.push(line);
    cursor = start + newCount;
    hunks++;
  }
  if (!hunks) throw invalid();
  for (let line = cursor; line < source.length; line++)
    restored.push(source[line]);
  return restored.join("");
}
