// CSS tokens are the browser's source of truth. This fallback also supports
// server rendering and editor settings before a stylesheet is available.
export const MONOSPACE_FONT_FALLBACK =
  "ui-monospace, SFMono-Regular, Consolas, monospace";

export function monospaceFont(element?: Element): string {
  const root = element ?? globalThis.document?.documentElement;
  if (!root || typeof getComputedStyle !== "function")
    return MONOSPACE_FONT_FALLBACK;
  return (
    getComputedStyle(root).getPropertyValue("--ui-font-mono").trim() ||
    MONOSPACE_FONT_FALLBACK
  );
}
