import { describe, expect, it } from "vitest";
import {
  moveGridSelection,
  radialGridOrder,
  moveRankedGridSelection,
} from "@lesomnus/cxz-ui";

describe("candidate grid navigation", () => {
  it("moves by physical cells and stays inside rows and incomplete edges", () => {
    expect(moveGridSelection(1, 8, 3, "ArrowRight")).toBe(2);
    expect(moveGridSelection(1, 8, 3, "ArrowLeft")).toBe(0);
    expect(moveGridSelection(1, 8, 3, "ArrowDown")).toBe(4);
    expect(moveGridSelection(4, 8, 3, "ArrowUp")).toBe(1);
    expect(moveGridSelection(2, 8, 3, "ArrowRight")).toBe(2);
    expect(moveGridSelection(3, 8, 3, "ArrowLeft")).toBe(3);
    expect(moveGridSelection(2, 8, 3, "ArrowUp")).toBe(2);
    expect(moveGridSelection(5, 8, 3, "ArrowDown")).toBe(7);
    expect(moveGridSelection(7, 8, 3, "ArrowDown")).toBe(7);
    expect(moveGridSelection(1, 3, 1, "ArrowDown")).toBe(2);
  });
});

describe("caret-centered relevance layout", () => {
  it("places the best candidates nearest either opening edge and leaves distant corners empty", () => {
    const above = radialGridOrder(8, [25, 75, 125], 20, 0, 8, {
      x: 75,
      edge: "bottom",
      distance: 14,
    });
    const below = radialGridOrder(8, [25, 75, 125], 20, 0, 8, {
      x: 75,
      edge: "top",
      distance: 14,
    });
    expect(above[7]).toBe(0);
    expect(below[1]).toBe(0);
    expect(above.filter((rank) => rank >= 0).sort((a, b) => a - b)).toEqual([
      0, 1, 2, 3, 4, 5, 6, 7,
    ]);
    expect(above.filter((rank) => rank < 0)).toHaveLength(1);
    expect(radialGridOrder(0, [25], 20, 0, 8, { x: 0, edge: "top" })).toEqual(
      [],
    );
    const left = radialGridOrder(8, [25, 75, 125], 20, 0, 8, {
      x: 0,
      edge: "bottom",
    });
    const right = radialGridOrder(8, [25, 75, 125], 20, 0, 8, {
      x: 150,
      edge: "bottom",
    });
    expect(left[6]).toBe(0);
    expect(right[8]).toBe(0);
  });
  it("navigates the physical matrix with ranked entries and empty corners", () => {
    const order = [-1, 7, 6, 5, 1, 4, 2, 0, 3];
    expect(moveRankedGridSelection(0, order, 3, "ArrowUp")).toBe(1);
    expect(moveRankedGridSelection(0, order, 3, "ArrowLeft")).toBe(2);
    expect(moveRankedGridSelection(0, order, 3, "ArrowRight")).toBe(3);
    expect(moveRankedGridSelection(0, order, 3, "ArrowDown")).toBe(0);
    expect(moveRankedGridSelection(5, order, 3, "ArrowUp")).toBe(7);
    expect(moveRankedGridSelection(7, order, 3, "ArrowLeft")).toBe(7);
    expect(moveRankedGridSelection(7, order, 3, "ArrowRight")).toBe(6);
  });
});
