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
