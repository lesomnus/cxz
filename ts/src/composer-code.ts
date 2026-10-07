import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import cpp from "highlight.js/lib/languages/cpp";
import css from "highlight.js/lib/languages/css";
import go from "highlight.js/lib/languages/go";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import markdown from "highlight.js/lib/languages/markdown";
import python from "highlight.js/lib/languages/python";
import rust from "highlight.js/lib/languages/rust";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";
import { expandPastes, type ComposerPaste } from "./composer-pastes";

const grammars = {
  bash,
  cpp,
  css,
  go,
  javascript,
  json,
  markdown,
  python,
  rust,
  sql,
  typescript,
  xml,
  yaml,
};
for (const [name, grammar] of Object.entries(grammars))
  hljs.registerLanguage(name, grammar);

export const codeSyntaxes = ["auto", "plaintext", ...Object.keys(grammars)];
const aliases: Record<string, string> = {
  js: "javascript",
  ts: "typescript",
  jsx: "javascript",
  tsx: "typescript",
  py: "python",
  sh: "bash",
  shell: "bash",
  html: "xml",
  yml: "yaml",
  md: "markdown",
  rs: "rust",
  c: "cpp",
  text: "plaintext",
  txt: "plaintext",
};
export function codeSyntax(syntax: string) {
  const name = syntax.toLowerCase();
  return aliases[name] ?? name;
}
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

const detections = new Map<string, string>();
export function detectCodeSyntax(body: string) {
  // Bound auto-detection work on large pasted snippets. Ambiguous/empty text is
  // explicitly plaintext; the user can always choose a language themselves.
  const sample = body.slice(0, 1024);
  if (!/[^\p{L}\p{N}_\s]/u.test(sample)) return "plaintext";
  const cached = detections.get(sample);
  if (cached) return cached;
  const syntax =
    hljs.highlightAuto(sample, Object.keys(grammars)).language ?? "plaintext";
  if (detections.size >= 32) detections.delete(detections.keys().next().value!);
  detections.set(sample, syntax);
  return syntax;
}

export function resolvedCodeSyntax(
  block: CodeBlock,
  text: string,
  pastes: Map<string, ComposerPaste>,
) {
  const syntax = codeSyntax(block.syntax);
  return syntax === "auto"
    ? detectCodeSyntax(
        expandPastes(text.slice(block.bodyStart, block.bodyEnd), pastes),
      )
    : syntax;
}

export function composerPrompt(
  text: string,
  pastes: Map<string, ComposerPaste>,
) {
  let result = "",
    offset = 0;
  for (const block of codeBlocks(text)) {
    const syntax = resolvedCodeSyntax(block, text, pastes);
    const header = text.slice(block.start, block.headerEnd);
    const info = header.slice(block.indent.length + block.fence.length).trim();
    const suffix = info.replace(/^\S+/, "");
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
      syntax +
      suffix +
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

export type CodeToken = { text: string; className: string };
export function highlightCodeLines(
  body: string,
  syntax: string,
): CodeToken[][] {
  if (
    !hljs.getLanguage(syntax) ||
    body.length > 65536 ||
    body.split("\n").some((line) => line.length > 2048)
  )
    return body.split("\n").map((text) => [{ text, className: "" }]);
  const html = hljs.highlight(body, {
    language: syntax,
    ignoreIllegals: true,
  }).value;
  const lines: CodeToken[][] = [[]];
  const scopes: string[] = [];
  // Consume only highlighter-generated spans and escaped text; render tokens
  // through React, never inject user-authored HTML into the input surface.
  for (const match of html.matchAll(
    /<span class="([^"]*)">|<\/span>|([^<]+)/g,
  )) {
    if (match[1] !== undefined) scopes.push(match[1]);
    else if (match[0] === "</span>") scopes.pop();
    else {
      const text = match[2].replace(
        /&(amp|lt|gt|quot|#x27);/g,
        (_, entity: string) =>
          ({ amp: "&", lt: "<", gt: ">", quot: '"', "#x27": "'" })[entity]!,
      );
      text.split("\n").forEach((part, index) => {
        if (index) lines.push([]);
        if (part)
          lines.at(-1)!.push({ text: part, className: scopes.join(" ") });
      });
    }
  }
  return lines;
}
