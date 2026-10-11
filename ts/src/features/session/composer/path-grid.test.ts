import { describe, expect, it } from "vitest";
import { moveGridSelection } from "@lesomnus/cxz-ui";

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
