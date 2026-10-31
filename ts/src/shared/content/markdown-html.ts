import { Renderer } from "marked";
import { conversationMarkdown } from "./markdown-parser";
import DOMPurify from "dompurify";
import { fileChipClassName } from "@lesomnus/nox";
import styles from "./markdown-code-block.module.css";
import { workspaceFilePath } from "./file-links";

// Keep conversation link behavior scoped to this sanitizer instance.
function purifier(fileLinks: boolean) {
  const instance = DOMPurify();
  if (fileLinks)
    instance.addHook("beforeSanitizeAttributes", (node) => {
      if (node.localName !== "a") return;
      const href = node.getAttribute("href") ?? "";
      if (!/^file:/i.test(href)) return;
      const path = workspaceFilePath(href);
      if (path === undefined) return;
      // Keep DOMPurify's protocol policy intact; only verified container files
      // become absolute-path links handled by the same click callback.
      node.setAttribute(
        "href",
        path.split("/").map(encodeURIComponent).join("/"),
      );
    });
  instance.addHook("afterSanitizeAttributes", (node) => {
    if (node.localName === "a" && node.hasAttribute("href")) {
      if (
        fileLinks &&
        workspaceFilePath(node.getAttribute("href")!) !== undefined
      ) {
        node.removeAttribute("target");
        node.removeAttribute("rel");
        node.classList.add(fileChipClassName);
        node.setAttribute("data-file-chip", "");
        node.setAttribute("data-container-file", "true");
        return;
      }
      node.setAttribute("target", "_blank");
      node.setAttribute("rel", "noopener noreferrer");
    }
  });
  return instance;
}
const markdownPurifiers = [purifier(false), purifier(true)];

// Virtual rows are destroyed outside the window. Retain only sanitized strings,
// never DOM or component state, with both an entry and text-storage budget.
const MAX_ENTRIES = 256;
const MAX_TEXT_BYTES = 2 * 1024 * 1024;
const rendered = new Map<string, { html: { __html: string }; bytes: number }>();
let retainedBytes = 0;

export function markdownHTML(text: string, fileLinks: boolean) {
  const key = `${fileLinks ? "files" : "web"}:${text}`;
  const cached = rendered.get(key);
  if (cached) {
    rendered.delete(key);
    rendered.set(key, cached);
    return cached.html;
  }
  const renderer = new Renderer();
  const code = renderer.code.bind(renderer);
  renderer.code = (token) =>
    `<div class="${styles.block}">${code(token)}</div>`;
  const html = {
    __html: markdownPurifiers[fileLinks ? 1 : 0]!.sanitize(
      conversationMarkdown.parse(text, { async: false, renderer }),
      {
        FORBID_TAGS: ["img", "style", "input", "form"],
        FORBID_ATTR: ["style"],
      },
    ),
  };
  const bytes = 2 * (key.length + html.__html.length);
  if (bytes <= MAX_TEXT_BYTES) {
    while (
      rendered.size >= MAX_ENTRIES ||
      retainedBytes + bytes > MAX_TEXT_BYTES
    ) {
      const oldest = rendered.keys().next().value!;
      retainedBytes -= rendered.get(oldest)!.bytes;
      rendered.delete(oldest);
    }
    rendered.set(key, { html, bytes });
    retainedBytes += bytes;
  }
  return html;
}
