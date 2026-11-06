import { useLayoutEffect, useRef, type RefObject } from "react";
import { controlCharacters } from "#src/shared/content/control-characters.tsx";
import styles from "./textarea-control-characters.module.css";

/** Non-interactive marks follow the native textarea's font, wrapping and scroll. */
export function TextareaControlCharacters({
  textarea,
  value,
  layoutKey,
}: {
  textarea: RefObject<HTMLTextAreaElement | null>;
  value: string;
  layoutKey?: unknown;
}) {
  const surface = useRef<HTMLDivElement>(null);
  const mirror = useRef<HTMLDivElement>(null);
  const synchronize = useRef(() => {});
  useLayoutEffect(() => {
    const input = textarea.current!;
    const frame = surface.current!;
    const content = mirror.current!;
    function scroll() {
      content.style.transform = `translate(${-input.scrollLeft}px, ${-input.scrollTop}px)`;
    }
    function resize() {
      const computed = getComputedStyle(input);
      for (const property of [
        "fontFamily",
        "fontSize",
        "fontWeight",
        "fontStyle",
        "fontVariant",
        "lineHeight",
        "letterSpacing",
        "wordSpacing",
        "whiteSpace",
        "wordBreak",
        "overflowWrap",
        "tabSize",
        "textAlign",
        "textIndent",
        "direction",
        "paddingTop",
        "paddingRight",
        "paddingBottom",
        "paddingLeft",
      ] as const) {
        content.style[property] = computed[property];
      }
      // clientWidth/Height round to integers; retain fractional layout widths
      // while subtracting the native border and scrollbar reservation.
      const pixels = (value: string) => parseFloat(value) || 0;
      const borderBox = computed.boxSizing === "border-box";
      const width =
        pixels(computed.width) +
        (borderBox
          ? 0
          : pixels(computed.paddingLeft) +
            pixels(computed.paddingRight) +
            pixels(computed.borderLeftWidth) +
            pixels(computed.borderRightWidth));
      const height =
        pixels(computed.height) +
        (borderBox
          ? 0
          : pixels(computed.paddingTop) +
            pixels(computed.paddingBottom) +
            pixels(computed.borderTopWidth) +
            pixels(computed.borderBottomWidth));
      frame.style.left = `${input.offsetLeft + pixels(computed.borderLeftWidth)}px`;
      frame.style.top = `${input.offsetTop + pixels(computed.borderTopWidth)}px`;
      frame.style.width = `${Math.max(0, width - input.offsetWidth + input.clientWidth)}px`;
      frame.style.height = `${Math.max(0, height - input.offsetHeight + input.clientHeight)}px`;
      scroll();
    }
    synchronize.current = resize;
    const observer = new ResizeObserver(resize);
    observer.observe(input);
    input.addEventListener("scroll", scroll, { passive: true });
    document.fonts.addEventListener("loadingdone", resize);
    resize();
    return () => {
      observer.disconnect();
      input.removeEventListener("scroll", scroll);
      document.fonts.removeEventListener("loadingdone", resize);
      synchronize.current = () => {};
    };
  }, [textarea]);
  useLayoutEffect(() => synchronize.current(), [value, layoutKey]);
  return (
    <div
      ref={surface}
      className={styles.surface}
      aria-hidden="true"
      data-control-character-overlay
    >
      <div ref={mirror} className={styles.mirror}>
        {controlCharacters(value.replace(/\r\n?/g, "\n"))}
      </div>
    </div>
  );
}
