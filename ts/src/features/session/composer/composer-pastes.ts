export type ComposerPaste = {
  token: string;
  body: string;
  lines: number;
  bytes: number;
  attachment?: {
    file: File;
    name: string;
    directory?: boolean;
    state: "uploading" | "ready" | "error";
    path?: string;
    error?: string;
    listeners?: Set<() => void>;
  };
};
export type PasteRange = {
  start: number;
  end: number;
  paste: ComposerPaste;
};
export const MAX_PASTE_BYTES = 1024 * 1024;
export const MAX_PASTE_CACHE_BYTES = 32 * MAX_PASTE_BYTES;

export function needsPasteChip(text: string) {
  let characters = 0,
    newlines = 0;
  for (const character of text) {
    if (++characters > 800 || (character === "\n" && ++newlines >= 3))
      return true;
  }
  return false;
}

export function createPaste(text: string, id: string): ComposerPaste {
  const bytes = new TextEncoder().encode(text).length;
  const lines = text.split("\n").length;
  return {
    token: `[Paste ${id} · ${lines}L · ${bytes}B]`,
    body: text,
    lines,
    bytes,
  };
}

export function pasteRanges(text: string, pastes: Map<string, ComposerPaste>) {
  const ranges: PasteRange[] = [];
  for (const match of text.matchAll(
    /\[(?:Paste [0-9a-f]{8} · \d+L · \d+B|File [0-9a-f]{8} · [^\]\r\n]+)\]/g,
  )) {
    const paste = pastes.get(match[0]);
    if (paste)
      ranges.push({
        start: match.index,
        end: match.index + match[0].length,
        paste,
      });
  }
  return ranges;
}

// One pass: original pasted text can itself contain a label from another chip.
export function expandPastes(
  text: string,
  pastes: Map<string, ComposerPaste>,
  attachmentPaths = true,
) {
  let result = "",
    offset = 0;
  for (const range of pasteRanges(text, pastes)) {
    const attachment = range.paste.attachment;
    const body =
      attachmentPaths && attachment?.path
        ? attachment.directory
          ? `[Attached directory archive: ${attachment.path} — extract this tar archive to read the directory contents]`
          : `[Attached file: ${attachment.path} — read this file for the full content]`
        : range.paste.body;
    result += text.slice(offset, range.start) + body;
    offset = range.end;
  }
  return result + text.slice(offset);
}

export function attachmentsReady(
  text: string,
  pastes: Map<string, ComposerPaste>,
) {
  return pasteRanges(text, pastes).every(
    ({ paste }) => !paste.attachment || paste.attachment.state === "ready",
  );
}

export function createFileChip(
  file: File,
  name: string,
  directory = false,
): ComposerPaste {
  const id = crypto.randomUUID().replaceAll("-", "").slice(0, 8);
  const label = name.replace(/[\[\]\r\n\x00-\x1f\x7f]/g, "_");
  return {
    token: `[File ${id} · ${label}]`,
    body: "",
    lines: 0,
    bytes: file.size,
    attachment: { file, name, directory, state: "uploading" },
  };
}

export function wholePasteSelection(
  ranges: PasteRange[],
  start: number,
  end: number,
) {
  for (const range of ranges) {
    if (start === end && start > range.start && start < range.end) {
      start = end = range.end;
    } else if (start < range.end && end > range.start) {
      start = Math.min(start, range.start);
      end = Math.max(end, range.end);
    }
  }
  return { start, end };
}

export function partialPasteEdit(
  before: string,
  after: string,
  pastes: Map<string, ComposerPaste>,
) {
  if (before === after) return false;
  let start = 0,
    oldEnd = before.length,
    newEnd = after.length;
  while (start < oldEnd && start < newEnd && before[start] === after[start])
    start++;
  while (
    oldEnd > start &&
    newEnd > start &&
    before[oldEnd - 1] === after[newEnd - 1]
  ) {
    oldEnd--;
    newEnd--;
  }
  return pasteRanges(before, pastes).some(
    ({ start: a, end: b }) =>
      (start < b && oldEnd > a && !(start <= a && oldEnd >= b)) ||
      (start === oldEnd && start > a && start < b),
  );
}
