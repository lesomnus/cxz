export type EditorSelection = {
  start: number;
  end: number;
  direction: "forward" | "backward" | "none";
  scrollTop: number;
  scrollLeft: number;
};

export function validEditorSelection(value: unknown): value is EditorSelection {
  if (!value || typeof value !== "object") return false;
  const v = value as EditorSelection;
  return (
    Number.isSafeInteger(v.start) &&
    v.start >= 0 &&
    Number.isSafeInteger(v.end) &&
    v.end >= v.start &&
    ["forward", "backward", "none"].includes(v.direction) &&
    Number.isFinite(v.scrollTop) &&
    v.scrollTop >= 0 &&
    Number.isFinite(v.scrollLeft) &&
    v.scrollLeft >= 0
  );
}

export function restoreEditorSelection(
  value: EditorSelection | undefined,
  length: number,
): EditorSelection {
  return value
    ? {
        ...value,
        start: Math.min(length, value.start),
        end: Math.min(length, value.end),
      }
    : {
        start: length,
        end: length,
        direction: "none",
        scrollTop: 0,
        scrollLeft: 0,
      };
}
