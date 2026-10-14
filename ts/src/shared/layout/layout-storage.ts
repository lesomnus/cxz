// Browser-local geometry is separate from the user-editable settings document.
export const LAYOUT_STORAGE_KEY = "cxz.layout.v1";
export type Pane = "session" | "file";

function read(storage: Storage): Record<string, unknown> {
  const value = JSON.parse(storage.getItem(LAYOUT_STORAGE_KEY) ?? "{}");
  return value && typeof value === "object" && !Array.isArray(value)
    ? value
    : {};
}
export function loadPaneWidth(pane: Pane): number | undefined {
  try {
    const value = read(localStorage)[pane];
    return typeof value === "number" && Number.isFinite(value) && value > 0
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}
export function savePaneWidth(pane: Pane, value: number | undefined) {
  try {
    let sizes: Record<string, unknown>;
    try {
      sizes = read(localStorage);
    } catch {
      sizes = {};
    }
    if (value === undefined) delete sizes[pane];
    else if (Number.isFinite(value) && value > 0) sizes[pane] = value;
    else return;
    localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(sizes));
  } catch {
    // Resizing still works when browser storage is denied or full.
  }
}
