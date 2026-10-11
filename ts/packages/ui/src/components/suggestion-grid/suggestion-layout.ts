export type SuggestionAnchor = {
  /** Horizontal caret position relative to the grid, in viewport pixels. */
  x: number;
  edge: "top" | "bottom";
  /** Distance between the caret and the menu's nearest edge. */
  distance?: number;
};

/** Map visible cells to relevance ranks; distant corners can remain empty. */
export function radialGridOrder(
  count: number,
  centers: readonly number[],
  rowHeight: number,
  rowGap: number,
  padding: number,
  anchor: SuggestionAnchor,
) {
  if (!count || !centers.length) return [];
  const columns = centers.length,
    rows = Math.ceil(count / columns);
  const cells = Array.from({ length: rows * columns }, (_, cell) => {
    const row = Math.floor(cell / columns),
      column = cell % columns;
    const vertical =
      (anchor.edge === "bottom" ? rows - row - 1 : row) * (rowHeight + rowGap) +
      rowHeight / 2 +
      padding +
      (anchor.distance ?? 0);
    return {
      cell,
      distance: (centers[column] - anchor.x) ** 2 + vertical ** 2,
    };
  }).sort((a, b) => a.distance - b.distance || a.cell - b.cell);
  const order = Array<number>(rows * columns).fill(-1);
  cells.slice(0, count).forEach(({ cell }, rank) => {
    order[cell] = rank;
  });
  return order;
}

/** Spatial navigation follows the rendered matrix rather than relevance rank. */
export function moveRankedGridSelection(
  rank: number,
  order: readonly number[],
  columns: number,
  key: string,
) {
  const cell = order.indexOf(rank);
  if (cell < 0 || columns < 1) return rank;
  const row = Math.floor(cell / columns),
    column = cell % columns;
  if (key === "ArrowLeft" || key === "ArrowRight") {
    const step = key === "ArrowLeft" ? -1 : 1;
    for (let next = column + step; next >= 0 && next < columns; next += step) {
      const candidate = order[row * columns + next];
      if (candidate >= 0) return candidate;
    }
  } else if (key === "ArrowUp" || key === "ArrowDown") {
    const step = key === "ArrowUp" ? -1 : 1,
      rows = Math.ceil(order.length / columns);
    for (let next = row + step; next >= 0 && next < rows; next += step) {
      const candidates = order
        .slice(next * columns, (next + 1) * columns)
        .map((value, col) => ({ value, distance: Math.abs(col - column) }))
        .filter((item) => item.value >= 0)
        .sort((a, b) => a.distance - b.distance);
      if (candidates.length) return candidates[0].value;
    }
  }
  return rank;
}
