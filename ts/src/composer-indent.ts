// Return one native text replacement so indentation keeps Undo/Redo atomic.
export function indentEdit(
  value: string,
  start: number,
  end: number,
  outdent: boolean,
) {
  if (!outdent && start === end)
    return { from: start, to: end, text: "  ", start: start + 2, end: end + 2 };

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
        ? (/^(?:\t| {1,2})/.exec(line)?.[0].length ?? 0)
        : 0;
      const added = outdent ? 0 : 2;
      edits.push({ position: offset, removed, added });
      offset += line.length + 1;
      return (outdent ? "" : "  ") + line.slice(removed);
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
