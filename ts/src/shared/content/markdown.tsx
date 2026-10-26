import { Renderer } from "marked";
import { conversationMarkdown } from "./markdown-parser";
import DOMPurify from "dompurify";
import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { MarkdownCodeBlock } from "./markdown-code-block";
import styles from "./markdown-code-block.module.css";
import { workspaceFilePath } from "./file-links";

export const MarkdownFileLinks = createContext<
  ((path: string) => void) | undefined
>(undefined);

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
        return;
      }
      node.setAttribute("target", "_blank");
      node.setAttribute("rel", "noopener noreferrer");
    }
  });
  return instance;
}
const markdownPurifiers = [purifier(false), purifier(true)];

export function Markdown({
  text,
  surface = "default",
}: {
  text: string;
  surface?: "default" | "input";
}) {
  useLocale();
  const openFile = useContext(MarkdownFileLinks);
  const fileLinks = !!openFile;
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
      __html: markdownPurifiers[fileLinks ? 1 : 0]!.sanitize(
        conversationMarkdown.parse(text, { async: false, renderer }),
        {
          FORBID_TAGS: ["img", "style", "input", "form"],
          FORBID_ATTR: ["style"],
        },
      ),
    };
  }, [text, fileLinks]);
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
      <div
        ref={root}
        className="markdown"
        dangerouslySetInnerHTML={html}
        onClick={(event) => {
          if (!openFile || event.button !== 0) return;
          const anchor = (event.target as Element).closest("a[href]");
          if (!anchor || !event.currentTarget.contains(anchor)) return;
          const path = workspaceFilePath(anchor.getAttribute("href")!);
          if (path === undefined) return;
          event.preventDefault();
          openFile(path);
        }}
      />
      {blocks.map((block, index) => (
        <MarkdownCodeBlock key={index} {...block} surface={surface} />
      ))}
    </>
  );
}
