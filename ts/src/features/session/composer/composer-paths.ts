import type { ProjectPathEntry } from "#gen/cxz/project_svc_pb";
import { codeBlocks } from "./composer-code";
import { matchCommands, type CommandMatch } from "./composer-commands";
import { t } from "#src/shared/i18n/i18n.ts";

export type LookupPaths = (
  directory: string,
  signal: AbortSignal,
) => Promise<ProjectPathEntry[]>;
export type PathQuery = {
  start: number;
  end: number;
  text: string;
  parent: string;
  query: string;
};
export type PathMatch = CommandMatch & { entry: ProjectPathEntry };

export function pathKind(entry: ProjectPathEntry) {
  if (entry.symlink) return "symlink";
  if (entry.directory) return "directory";
  if (!entry.kind) return "file";
  return [
    "file",
    "directory",
    "symlink",
    "fifo",
    "socket",
    "block",
    "character",
  ].includes(entry.kind)
    ? entry.kind
    : "unknown";
}

export function pathDescription(entry: ProjectPathEntry) {
  const labels: Record<string, string> = {
    directory: t("Directory"),
    file: t("File"),
    symlink: t("Symbolic link"),
    fifo: t("Pipe"),
    socket: t("Socket"),
    block: t("Block device"),
    character: t("Character device"),
    unknown: t("Unknown file type"),
  };
  const kind = pathKind(entry);
  return (
    (labels[kind] ?? labels.unknown) +
    (entry.executable ? ` · ${t("Executable")}` : "")
  );
}

function pathQuery(
  value: string,
  start: number,
  end: number,
  caret: number,
  workspace: string,
): PathQuery | undefined {
  const text = value.slice(start, caret);
  if (/[\r\n\0]/.test(text) || text.startsWith("--")) return;
  let parent: string, query: string;
  if (text === "~") {
    parent = "~/";
    query = "";
  } else {
    if (text.startsWith("~") && !text.startsWith("~/")) return;
    const slash = text.lastIndexOf("/");
    parent = slash < 0 ? "" : text.slice(0, slash + 1);
    query = text.slice(slash + 1);
  }
  const directory =
    parent.startsWith("/") || parent.startsWith("~/")
      ? parent
      : workspace
        ? workspace.replace(/\/$/, "") + "/" + parent
        : "";
  if (!directory) return;
  return { start, end, text, parent: directory, query };
}

export function containerPathQuery(
  value: string,
  caret: number,
  end = caret,
  workspace = "",
) {
  if (caret !== end) return;
  const download = /^\/download[ \t]+/.exec(value);
  if (download && !/[\r\n]/.test(value)) {
    let start = download[0].length,
      finish = value.length;
    const quote = value[start];
    if (["`", '"', "'"].includes(quote)) {
      start++;
      if (value[finish - 1] === quote) finish--;
    }
    if (caret < start || caret > finish) return;
    return pathQuery(value, start, finish, caret, workspace);
  }
  if (
    codeBlocks(value).some(
      (block) => caret >= block.start && caret <= block.end,
    )
  )
    return;
  const startLine = value.lastIndexOf("\n", Math.max(0, caret - 1)) + 1;
  const endLine = value.indexOf("\n", caret);
  const line = value.slice(startLine, endLine < 0 ? value.length : endLine);
  const runs = [...line.matchAll(/`+/g)];
  for (let i = 0; i < runs.length; i++) {
    const run = runs[i];
    let escapes = 0;
    for (let j = run.index - 1; j >= 0 && line[j] === "\\"; j--) escapes++;
    if (escapes % 2) continue;
    const close = runs.findIndex(
      (next, index) => index > i && next[0].length === run[0].length,
    );
    const finish = close < 0 ? line.length : runs[close].index;
    const start = startLine + run.index + run[0].length;
    if (run[0].length === 1 && caret >= start && caret <= startLine + finish) {
      const text = value.slice(start, caret);
      if (text.startsWith("/") || text === "~" || text.startsWith("~/"))
        return pathQuery(value, start, startLine + finish, caret, workspace);
    }
    if (close >= 0) i = close;
  }
}

export function matchPaths(
  entries: readonly ProjectPathEntry[],
  token: PathQuery,
): PathMatch[] {
  const valid = entries.filter(
    (entry) =>
      entry.name &&
      ![".", ".."].includes(entry.name) &&
      !/[\/\0\r\n`]/.test(entry.name),
  );
  const matched = matchCommands(
    valid.map((entry) => ({
      name: entry.name,
      description: pathDescription(entry),
    })),
    token.query,
  );
  const byName = new Map(valid.map((entry) => [entry.name, entry]));
  return matched
    .sort(
      (a, b) =>
        a.score - b.score ||
        Number(byName.get(b.command.name)!.directory) -
          Number(byName.get(a.command.name)!.directory) ||
        a.command.name.localeCompare(b.command.name),
    )
    .map((match) => ({
      ...match,
      entry: byName.get(match.command.name)!,
      positions: match.positions.map(
        (position) => position + token.parent.length,
      ),
      command: {
        ...match.command,
        name:
          token.parent +
          match.command.name +
          (byName.get(match.command.name)!.directory ? "/" : ""),
      },
    }));
}

export function downloadArgument(value: string) {
  if (!/^\/download(?:[ \t]|$)/.test(value)) return undefined;
  if (/[\0\r\n]/.test(value))
    throw new Error(t("Choose a container file: /download <path>"));
  let path = value.slice("/download".length).trim();
  if (
    path.length >= 2 &&
    ["`", '"', "'"].includes(path[0]) &&
    path.at(-1) === path[0]
  )
    path = path.slice(1, -1);
  if (!path || path.length > 4096 || /[\0\r\n]/.test(path))
    throw new Error(t("Choose a container file: /download <path>"));
  return path;
}
