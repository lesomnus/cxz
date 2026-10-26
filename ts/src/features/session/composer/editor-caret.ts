// Measure the native caret's wrapped line without altering the input or its
// selection. The offscreen mirror uses the textarea's actual font and width.
export function caretPosition(input: HTMLTextAreaElement, offset: number) {
  const style = getComputedStyle(input);
  const mirror = document.createElement("div");
  Object.assign(mirror.style, {
    position: "fixed",
    visibility: "hidden",
    left: "-10000px",
    top: "0",
    width: style.width,
    boxSizing: style.boxSizing,
    border: style.border,
    overflow: "hidden",
    scrollbarGutter: style.scrollbarGutter,
    scrollbarWidth: style.scrollbarWidth,
    whiteSpace: style.whiteSpace,
    overflowWrap: style.overflowWrap,
    wordBreak: style.wordBreak,
    lineBreak: style.lineBreak,
    fontFamily: style.fontFamily,
    fontSize: style.fontSize,
    fontWeight: style.fontWeight,
    fontStyle: style.fontStyle,
    fontStretch: style.fontStretch,
    fontVariant: style.fontVariant,
    fontKerning: style.fontKerning,
    fontVariantLigatures: style.fontVariantLigatures,
    fontFeatureSettings: style.fontFeatureSettings,
    fontVariationSettings: style.fontVariationSettings,
    wordSpacing: style.wordSpacing,
    lineHeight: style.lineHeight,
    letterSpacing: style.letterSpacing,
    tabSize: style.tabSize,
    padding: style.padding,
    textIndent: style.textIndent,
  });
  mirror.textContent = input.value.slice(0, offset);
  const marker = document.createElement("span");
  marker.textContent = input.value[offset] || "\u200b";
  Object.assign(marker.style, {
    display: "inline-block",
    height: style.lineHeight,
    verticalAlign: "top",
  });
  mirror.append(marker);
  document.body.append(mirror);
  const point = marker.getBoundingClientRect(),
    bounds = mirror.getBoundingClientRect();
  const position = {
    top: point.top - bounds.top,
    left: point.left - bounds.left,
  };
  mirror.remove();
  return position;
}
