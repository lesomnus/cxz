// Keep the original character as text. Markers are CSS pseudo-elements, so they
// add no editable/copied text and do not change tab stops or line wrapping.
export function controlCharacters(line: string) {
  return line
    .split(/([ \u0000-\u001f\u007f\u00a0\u200b])/u)
    .map((part, index) => {
      const symbol =
        part === " " || part === "\u00a0"
          ? "·"
          : part === "\t"
            ? "→"
            : part === "\u200b"
              ? "¦"
              : part.length === 1 && part.charCodeAt(0) < 32
                ? String.fromCodePoint(0x2400 + part.charCodeAt(0))
                : part === "\u007f"
                  ? "␡"
                  : undefined;
      return symbol ? (
        <span
          key={index}
          className="editor-control-character"
          data-symbol={symbol}
        >
          {part}
        </span>
      ) : (
        part
      );
    });
}
