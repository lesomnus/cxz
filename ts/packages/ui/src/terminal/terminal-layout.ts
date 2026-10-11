import type { SplitNode } from "../components/split-layout/split-layout";

export type DockEdge = "center" | "left" | "right" | "top" | "bottom";
export type TerminalGroup = { id: string; tabs: string[]; active: string };
export type TerminalLayout = {
  tree: SplitNode | null;
  groups: TerminalGroup[];
};

export function initialTerminalLayout(): TerminalLayout {
  return {
    tree: { kind: "pane", id: "group-1" },
    groups: [{ id: "group-1", tabs: ["terminal-1"], active: "terminal-1" }],
  };
}

function replacePane(tree: SplitNode, id: string, next: SplitNode): SplitNode {
  if (tree.kind === "pane") return tree.id === id ? next : tree;
  return {
    ...tree,
    first: replacePane(tree.first, id, next),
    second: replacePane(tree.second, id, next),
  };
}
function removePane(tree: SplitNode, id: string): SplitNode | null {
  if (tree.kind === "pane") return tree.id === id ? null : tree;
  const first = removePane(tree.first, id),
    second = removePane(tree.second, id);
  return first && second ? { ...tree, first, second } : (first ?? second);
}
export function paneOrder(tree: SplitNode | null): string[] {
  return !tree
    ? []
    : tree.kind === "pane"
      ? [tree.id]
      : [...paneOrder(tree.first), ...paneOrder(tree.second)];
}
export function selectTerminal(
  layout: TerminalLayout,
  tab: string,
): TerminalLayout {
  return {
    ...layout,
    groups: layout.groups.map((group) =>
      group.tabs.includes(tab) ? { ...group, active: tab } : group,
    ),
  };
}
export function closeTerminal(
  layout: TerminalLayout,
  tab: string,
): TerminalLayout {
  const group = layout.groups.find((group) => group.tabs.includes(tab));
  if (!group) return layout;
  const index = group.tabs.indexOf(tab);
  const tabs = group.tabs.filter((id) => id !== tab);
  if (!tabs.length)
    return {
      tree: layout.tree ? removePane(layout.tree, group.id) : null,
      groups: layout.groups.filter((item) => item.id !== group.id),
    };
  return {
    ...layout,
    groups: layout.groups.map((item) =>
      item.id === group.id
        ? {
            ...item,
            tabs,
            active:
              item.active === tab
                ? tabs[Math.min(index, tabs.length - 1)]!
                : item.active,
          }
        : item,
    ),
  };
}
export function addTerminal(
  layout: TerminalLayout,
  tab: string,
  groupId: string,
  edge: DockEdge = "center",
  newGroupId = `group-${tab}`,
): TerminalLayout {
  const target = layout.groups.find((group) => group.id === groupId);
  if (layout.tree && !target) return layout;
  if (!target || !layout.tree)
    return {
      tree: { kind: "pane", id: newGroupId },
      groups: [{ id: newGroupId, tabs: [tab], active: tab }],
    };
  if (edge === "center")
    return {
      ...layout,
      groups: layout.groups.map((group) =>
        group.id === groupId
          ? { ...group, tabs: [...group.tabs, tab], active: tab }
          : group,
      ),
    };
  const leaf: SplitNode = { kind: "pane", id: newGroupId };
  const original: SplitNode = { kind: "pane", id: target.id };
  const before = edge === "left" || edge === "top";
  return {
    tree: replacePane(layout.tree, groupId, {
      kind: "split",
      id: `split-${newGroupId}`,
      axis: edge === "left" || edge === "right" ? "horizontal" : "vertical",
      ratio: 0.5,
      first: before ? leaf : original,
      second: before ? original : leaf,
    }),
    groups: [...layout.groups, { id: newGroupId, tabs: [tab], active: tab }],
  };
}
export function moveTerminal(
  layout: TerminalLayout,
  tab: string,
  groupId: string,
  edge: DockEdge,
  newGroupId: string,
  index?: number,
): TerminalLayout {
  const source = layout.groups.find((group) => group.tabs.includes(tab));
  const target = layout.groups.find((group) => group.id === groupId);
  if (!source || !target) return layout;
  if (source === target && edge !== "center" && source.tabs.length === 1)
    return layout;
  if (edge === "center") {
    const adjusted =
      source === target &&
      index !== undefined &&
      source.tabs.indexOf(tab) < index
        ? index - 1
        : index;
    const tabs = target.tabs.filter((id) => id !== tab);
    tabs.splice(
      Math.max(0, Math.min(tabs.length, adjusted ?? tabs.length)),
      0,
      tab,
    );
    if (source === target)
      return {
        ...layout,
        groups: layout.groups.map((group) =>
          group.id === groupId ? { ...group, tabs, active: tab } : group,
        ),
      };
    const removed = closeTerminal(layout, tab);
    return {
      ...removed,
      groups: removed.groups.map((group) =>
        group.id === groupId ? { ...group, tabs, active: tab } : group,
      ),
    };
  }
  return addTerminal(
    closeTerminal(layout, tab),
    tab,
    groupId,
    edge,
    newGroupId,
  );
}
