// Return one native text replacement so indentation keeps Undo/Redo atomic.
export function indentEdit(
  value: string,
  start: number,
  end: number,
  outdent: boolean,
  options = { indentSize: 2, insertSpaces: true },
) {
  const indent = options.insertSpaces ? " ".repeat(options.indentSize) : "\t";
  if (!outdent && start === end)
    return {
      from: start,
      to: end,
      text: indent,
      start: start + indent.length,
      end: end + indent.length,
    };

  const from = start > 0 ? value.lastIndexOf("\n", start - 1) + 1 : 0;
  const last = end > start ? end - 1 : end;
  const newline = value.indexOf("\n", last);
  const to = newline < 0 ? value.length : newline;
  let offset = from;
  const edits: { position: number; removed: number; added: number }[] = [];
  const text = value
    .slice(from, to)
    .split("\n")
    .map((line) => {
      const removed = outdent
        ? (new RegExp(`^(?:\\t| {1,${options.indentSize}})`).exec(line)?.[0]
            .length ?? 0)
        : 0;
      const added = outdent ? 0 : indent.length;
      edits.push({ position: offset, removed, added });
      offset += line.length + 1;
      return (outdent ? "" : indent) + line.slice(removed);
    })
    .join("\n");
  function adjust(position: number) {
    let delta = 0;
    for (const edit of edits) {
      if (position < edit.position) break;
      if (position < edit.position + edit.removed)
        return edit.position + delta + edit.added;
      delta += edit.added - edit.removed;
    }
    return position + delta;
  }
  return { from, to, text, start: adjust(start), end: adjust(end) };
}
