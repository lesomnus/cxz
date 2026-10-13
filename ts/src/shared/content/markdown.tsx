import { marked, Renderer } from "marked";
import DOMPurify from "dompurify";
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { MarkdownCodeBlock } from "./markdown-code-block";
import styles from "./markdown-code-block.module.css";

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
  const root = useRef<HTMLDivElement>(null);
  const nativeCodes = useRef(
    new WeakMap<HTMLElement, { text: string; language: string }>(),
  );
  const html = useMemo(() => {
    const renderer = new Renderer();
    const code = renderer.code.bind(renderer);
    renderer.code = (token) =>
      `<div class="${styles.block}">${code(token)}</div>`;
    return {
      __html: markdownPurifier.sanitize(
        marked.parse(text, { async: false, renderer }),
        {
          FORBID_TAGS: ["img", "style", "input", "form"],
          FORBID_ATTR: ["style"],
        },
      ),
    };
  }, [text]);
  const [blocks, setBlocks] = useState<
    {
      container: HTMLElement;
      code: HTMLElement;
      text: string;
      language: string;
    }[]
  >([]);
  useLayoutEffect(() => {
    const blocks = [
      ...root.current!.querySelectorAll<HTMLElement>(
        `.${styles.block} > pre > code`,
      ),
    ].map((code) => {
      let native = nativeCodes.current.get(code);
      // React owns highlighted spans via a portal; only sanitized Markdown owns
      // the surrounding markup. Read literal code before handing over the node.
      if (!native) {
        native = {
          text: code.textContent ?? "",
          language:
            [...code.classList]
              .find((value) => value.startsWith("language-"))
              ?.slice(9) || "auto",
        };
        nativeCodes.current.set(code, native);
        code.replaceChildren();
      }
      return {
        container: code.parentElement!.parentElement!,
        code,
        ...native,
      };
    });
    setBlocks(blocks);
  }, [html]);
  return (
    <>
      <div ref={root} className="markdown" dangerouslySetInnerHTML={html} />
      {blocks.map((block, index) => (
        <MarkdownCodeBlock key={index} {...block} />
      ))}
    </>
  );
}
