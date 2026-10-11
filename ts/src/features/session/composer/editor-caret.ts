// Measure the native caret's wrapped line without altering the input or its
// selection. The offscreen mirror uses the textarea's actual font and width.
export function caretTop(input: HTMLTextAreaElement, offset: number) {
  const style = getComputedStyle(input);
  const mirror = document.createElement("div");
  Object.assign(mirror.style, {
    position: "fixed",
    visibility: "hidden",
    left: "-10000px",
    top: "0",
    width: `${input.clientWidth}px`,
    boxSizing: "border-box",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    font: style.font,
    lineHeight: style.lineHeight,
    letterSpacing: style.letterSpacing,
    tabSize: style.tabSize,
    padding: style.padding,
    textIndent: style.textIndent,
  });
  mirror.textContent = input.value.slice(0, offset);
  const marker = document.createElement("span");
  marker.textContent = input.value[offset] || ".";
  mirror.append(marker);
  document.body.append(mirror);
  const top =
    marker.getBoundingClientRect().top - mirror.getBoundingClientRect().top;
  mirror.remove();
  return top;
}
