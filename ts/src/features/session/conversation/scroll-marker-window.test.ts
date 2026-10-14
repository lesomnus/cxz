import { expect, it } from "vitest";
import {
  scrollMarkerWindow,
  type MarkerCoordinate,
} from "./scroll-marker-window";

it("keeps both conversation and search ticks on inclusive rail boundaries", () => {
  const markers: MarkerCoordinate[] = [
    { id: "1", kind: "input", y: 0 },
    { id: "search:1", kind: "search", y: 0 },
    { id: "2", kind: "assistant", y: 100 },
    { id: "search:2", kind: "search", y: 100 },
    { id: "3", kind: "assistant", y: 200 },
  ];
  expect(
    scrollMarkerWindow(
      markers,
      new Map(markers.map((x) => [x.id, x])),
      0,
      100,
    ).map((x) => x.id),
  ).toEqual(["1", "search:1", "2", "search:2"]);
});
it("retains outgoing animation ticks without mounting the history between distant windows", () => {
  let reads = 0;
  const markers: MarkerCoordinate[] = Array.from(
    { length: 8192 },
    (_, index) => ({
      id: String(index),
      kind: "assistant",
      get y() {
        reads++;
        return index * 100;
      },
    }),
  );
  const byId = new Map(markers.map((x) => [x.id, x]));
  const found = scrollMarkerWindow(
    markers,
    byId,
    100,
    300,
    new Map([
      ["8000", 2],
      ["8001", 3],
      ["removed-search", 4],
    ]),
  );
  expect(found.map((x) => x.id)).toEqual(["1", "2", "3", "8000", "8001"]);
  expect(reads).toBeLessThan(80);
  expect(scrollMarkerWindow([], new Map(), 0, 100)).toEqual([]);
});
