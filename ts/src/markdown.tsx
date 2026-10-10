import { marked } from "marked";
import DOMPurify from "dompurify";
import { useLocale } from "./i18n-react";

// Keep conversation link behavior scoped to this sanitizer instance.
const markdownPurifier = DOMPurify();
markdownPurifier.addHook("afterSanitizeAttributes", (node) => {
  if (node.localName === "a" && node.hasAttribute("href")) {
    node.setAttribute("target", "_blank");
    node.setAttribute("rel", "noopener noreferrer");
  }
});

export function Markdown({ text }: { text: string }) {
  useLocale();
  return (
    <div
      className="markdown"
      dangerouslySetInnerHTML={{
        __html: markdownPurifier.sanitize(
          marked.parse(text, { async: false }),
          {
            FORBID_TAGS: ["img", "style", "input", "form"],
            FORBID_ATTR: ["style"],
          },
        ),
      }}
    />
  );
}
