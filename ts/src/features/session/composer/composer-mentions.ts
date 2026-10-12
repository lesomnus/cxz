import type { Session } from "#gen/cxz/session_pb";
import { matchCommands } from "./composer-commands";
import { key } from "@lesomnus/payday/store";
import { codeBlocks } from "./composer-code";
function plainTextAtCaret(value: string, caret: number) {
  let offset = 0,
    outside = "";
  for (const block of codeBlocks(value)) {
    if (block.start <= caret && caret <= block.end) return false;
    if (block.start >= caret) break;
    outside += value.slice(offset, block.start);
    offset = block.end;
  }
  outside += value.slice(offset, caret);
  return (outside.match(/`/g)?.length ?? 0) % 2 === 0;
}

export function mentionQuery(value: string, start: number, end: number) {
  if (start !== end || value.startsWith("/")) return;
  const before = value.slice(0, start);
  const match = /(?:^|[\s([{])(@[A-Za-z0-9-]*)$/.exec(before);
  if (!match || !plainTextAtCaret(value, start)) return;
  const position = start - match[1].length;
  const tail = /^[A-Za-z0-9-]*/.exec(value.slice(start))![0];
  return { start: position, end: start + tail.length, query: match[1] };
}
export function redactQuery(value: string, start: number, end: number) {
  if (start !== end || value.startsWith("/")) return;
  const before = value.slice(0, start);
  const match = /(?:^|\s)(\/@[a-z]*)$/.exec(before);
  if (!match || !plainTextAtCaret(value, start)) return;
  return {
    start: start - match[1].length,
    end: start + (/^[a-z]*/.exec(value.slice(start))?.[0].length ?? 0),
    query: match[1],
  };
}
export function mentionMatches(
  sessions: Session[],
  current: Session,
  query: string,
) {
  return matchCommands(
    sessions
      .filter(
        (s) =>
          s.runtimeId !== current.runtimeId &&
          s.listed &&
          s.alias &&
          current.project?.id &&
          s.project?.id &&
          key(s.project.id) === key(current.project.id),
      )
      .map((s) => ({
        name: `@${s.alias}`,
        description: `${s.name || s.alias} · ${s.agent}`,
      })),
    query,
  );
}
