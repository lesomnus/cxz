import { expandPastes, type ComposerPaste } from "./composer-pastes";
import {
  codeSyntax,
  detectCodeSyntax,
} from "#src/shared/editor/code-syntax.ts";
export {
  codeSyntax,
  codeSyntaxes,
  detectCodeSyntax,
  highlightCodeLines,
  type CodeToken,
} from "#src/shared/editor/code-syntax.ts";

export type CodeBlock = {
  start: number;
  headerEnd: number;
  bodyStart: number;
  bodyEnd: number;
  end: number;
  startLine: number;
  endLine: number;
  fence: string;
  indent: string;
  syntax: string;
  closed: boolean;
};

// Keep the native draft as Markdown, so undo, selection and session drafts have
// a single source of truth. Fences of four or more backticks can contain ```.
export function codeBlocks(text: string): CodeBlock[] {
  const blocks: CodeBlock[] = [];
  let offset = 0;
  let open: CodeBlock | undefined;
  const lines = text.split("\n");
  lines.forEach((line, index) => {
    if (open) {
      const close = /^ {0,3}(`{3,})\s*$/.exec(line);
      if (close && close[1].length >= open.fence.length) {
        open.bodyEnd = offset;
        open.end = offset + line.length;
        open.endLine = index;
        open.closed = true;
        blocks.push(open);
        open = undefined;
      }
    } else {
      const start = /^( {0,3})(`{3,})([^`]*)$/.exec(line);
      if (start)
        open = {
          start: offset,
          headerEnd: offset + line.length,
          bodyStart: Math.min(text.length, offset + line.length + 1),
          bodyEnd: text.length,
          end: text.length,
          startLine: index,
          endLine: lines.length - 1,
          fence: start[2],
          indent: start[1],
          syntax: start[3].trim().split(/\s+/)[0] || "auto",
          closed: false,
        };
    }
    offset += line.length + 1;
  });
  if (open) blocks.push(open);
  return blocks;
}

export function resolvedCodeSyntax(
  block: CodeBlock,
  text: string,
  pastes: Map<string, ComposerPaste>,
) {
  const syntax = codeSyntax(block.syntax);
  return syntax === "auto"
    ? detectCodeSyntax(
        expandPastes(text.slice(block.bodyStart, block.bodyEnd), pastes, false),
      )
    : syntax;
}

// Presentation choices follow their opening fence through edits, but never
// become part of the draft or the prompt sent to the agent.
export function moveCodeSyntaxChoices(
  before: string,
  after: string,
  choices: ReadonlyMap<number, string>,
) {
  let prefix = 0,
    suffix = 0;
  while (
    prefix < Math.min(before.length, after.length) &&
    before[prefix] === after[prefix]
  )
    prefix++;
  while (
    suffix < Math.min(before.length, after.length) - prefix &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  )
    suffix++;
  const delta = after.length - before.length;
  const blocks = new Map(
    codeBlocks(after).map((block) => [block.start, block]),
  );
  const result = new Map<number, string>();
  for (const block of codeBlocks(before)) {
    const syntax = choices.get(block.start);
    if (!syntax) continue;
    const start =
      block.headerEnd < prefix
        ? block.start
        : block.start >= before.length - suffix
          ? block.start + delta
          : block.start < prefix
            ? block.start
            : undefined;
    const next = start === undefined ? undefined : blocks.get(start);
    if (
      next &&
      before.slice(block.start, block.headerEnd) ===
        after.slice(next.start, next.headerEnd)
    )
      result.set(next.start, syntax);
  }
  return result;
}

export function composerPrompt(
  text: string,
  pastes: Map<string, ComposerPaste>,
) {
  let result = "",
    offset = 0;
  for (const block of codeBlocks(text)) {
    const header = text.slice(block.start, block.headerEnd);
    const info = header
      .slice(block.indent.length + block.fence.length)
      .replace(/\r$/, "");
    const body = expandPastes(
      text.slice(block.bodyStart, block.bodyEnd),
      pastes,
    );
    // A chip can contain its own Markdown fence. Grow the enclosing fence so
    // its bytes remain code instead of terminating the surrounding block.
    let fenceLength = block.fence.length;
    for (const match of body.matchAll(/^ {0,3}(`{3,})[^\n`]*$/gm))
      fenceLength = Math.max(fenceLength, match[1].length + 1);
    const fence = "`".repeat(fenceLength);
    result +=
      expandPastes(text.slice(offset, block.start), pastes) +
      block.indent +
      fence +
      info +
      (header.endsWith("\r") ? "\r\n" : "\n") +
      body;
    if (!result.endsWith("\n")) result += "\n";
    result += block.closed
      ? text.slice(block.bodyEnd, block.end).replace(/`{3,}/, fence)
      : block.indent + fence;
    offset = block.end;
  }
  return result + expandPastes(text.slice(offset), pastes);
}
