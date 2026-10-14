export type MarkerCoordinate = {
  id: string;
  kind: "input" | "assistant" | "search";
  y: number;
};

// Coordinates are ordered by their row. Locate the current rail band without
// scanning the entire loaded history on every animation/scroll frame.
export function scrollMarkerWindow(
  coordinates: readonly MarkerCoordinate[],
  byId: ReadonlyMap<string, MarkerCoordinate>,
  low: number,
  high: number,
  outgoing?: ReadonlyMap<string, number>,
) {
  const bound = (value: number, inclusive: boolean) => {
    let left = 0,
      right = coordinates.length;
    while (left < right) {
      const middle = (left + right) >>> 1;
      if (
        coordinates[middle].y < value ||
        (inclusive && coordinates[middle].y === value)
      )
        left = middle + 1;
      else right = middle;
    }
    return left;
  };
  const current = coordinates.slice(bound(low, false), bound(high, true));
  if (!outgoing?.size) return current;
  const retained: MarkerCoordinate[] = [];
  for (const id of outgoing.keys()) {
    const marker = byId.get(id);
    if (marker && (marker.y < low || marker.y > high)) retained.push(marker);
  }
  retained.sort((a, b) => a.y - b.y);
  const merged: MarkerCoordinate[] = [];
  let left = 0,
    right = 0;
  while (left < current.length || right < retained.length)
    merged.push(
      right >= retained.length ||
        (left < current.length && current[left].y <= retained[right].y)
        ? current[left++]
        : retained[right++],
    );
  return merged;
}
