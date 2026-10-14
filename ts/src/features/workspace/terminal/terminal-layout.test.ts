import { describe, expect, it } from "vitest";
import {
  addTerminal,
  closeTerminal,
  initialTerminalLayout,
  moveTerminal,
  paneOrder,
  selectTerminal,
  type TerminalLayout,
} from "@lesomnus/nox/terminal";
import { setSplitRatio } from "@lesomnus/nox";

function invariant(layout: TerminalLayout) {
  const panes = paneOrder(layout.tree);
  expect(panes.sort()).toEqual(layout.groups.map((group) => group.id).sort());
  const tabs = layout.groups.flatMap((group) => group.tabs);
  expect(new Set(tabs).size).toBe(tabs.length);
  for (const group of layout.groups) {
    expect(group.tabs.length).toBeGreaterThan(0);
    expect(group.tabs).toContain(group.active);
  }
}
describe("terminal docking", () => {
  it("keeps tab identities when reordering and moving, collapsing an emptied nested pane", () => {
    let layout = initialTerminalLayout();
    layout = addTerminal(layout, "terminal-2", "group-1");
    layout = addTerminal(layout, "terminal-3", "group-1", "right", "right");
    layout = addTerminal(layout, "terminal-4", "right", "bottom", "bottom");
    invariant(layout);
    expect(paneOrder(layout.tree)).toEqual(["group-1", "right", "bottom"]);
    layout = moveTerminal(
      layout,
      "terminal-3",
      "group-1",
      "center",
      "unused",
      0,
    );
    invariant(layout);
    expect(paneOrder(layout.tree)).toEqual(["group-1", "bottom"]);
    expect(layout.groups[0]!.tabs).toEqual([
      "terminal-3",
      "terminal-1",
      "terminal-2",
    ]);
    layout = moveTerminal(
      layout,
      "terminal-3",
      "group-1",
      "center",
      "unused",
      3,
    );
    expect(layout.groups[0]!.tabs).toEqual([
      "terminal-1",
      "terminal-2",
      "terminal-3",
    ]);
    layout = moveTerminal(layout, "terminal-2", "bottom", "left", "left");
    invariant(layout);
    expect(paneOrder(layout.tree)).toEqual(["group-1", "left", "bottom"]);
    expect(layout.groups.find((group) => group.id === "left")!.active).toBe(
      "terminal-2",
    );
  });
  it("does not split a lone tab away from itself or lose tabs on an invalid drop", () => {
    const layout = initialTerminalLayout();
    expect(addTerminal(layout, "terminal-2", "missing")).toBe(layout);
    expect(moveTerminal(layout, "terminal-1", "group-1", "left", "new")).toBe(
      layout,
    );
    expect(moveTerminal(layout, "missing", "group-1", "center", "new")).toBe(
      layout,
    );
    expect(moveTerminal(layout, "terminal-1", "missing", "center", "new")).toBe(
      layout,
    );
  });
  it("selects a neighboring tab on close, removes the last pane, and can reopen", () => {
    let layout = addTerminal(initialTerminalLayout(), "terminal-2", "group-1");
    layout = selectTerminal(layout, "terminal-1");
    layout = closeTerminal(layout, "terminal-2");
    expect(layout.groups[0]!.active).toBe("terminal-1");
    layout = closeTerminal(layout, "terminal-1");
    expect(layout).toEqual({ tree: null, groups: [] });
    layout = addTerminal(layout, "terminal-3", "", "center", "new");
    invariant(layout);
    expect(layout.groups[0]!.tabs).toEqual(["terminal-3"]);
  });
  it("preserves independent nested split ratios and clamps resizing", () => {
    let layout = addTerminal(
      initialTerminalLayout(),
      "terminal-2",
      "group-1",
      "right",
      "right",
    );
    layout = addTerminal(layout, "terminal-3", "right", "bottom", "bottom");
    const tree = setSplitRatio(layout.tree!, "split-right", 0.7);
    const resized = setSplitRatio(tree, "split-bottom", -1);
    expect(resized.kind === "split" && resized.ratio).toBe(0.7);
    expect(
      resized.kind === "split" &&
        resized.second.kind === "split" &&
        resized.second.ratio,
    ).toBe(0.15);
    invariant({ ...layout, tree: resized });
  });
});
