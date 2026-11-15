export function sidebarPopupGap(anchor: HTMLElement) {
  const sidebar = anchor.closest(".resource-sidebar");
  if (!sidebar) return 4;
  const border = parseFloat(getComputedStyle(sidebar).borderRightWidth) || 0;
  const inset = Math.max(
    0,
    sidebar.getBoundingClientRect().right -
      border -
      anchor.getBoundingClientRect().right,
  );
  return 2 * inset + border;
}
