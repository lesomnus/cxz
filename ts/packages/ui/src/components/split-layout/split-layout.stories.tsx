import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SplitLayout, setSplitRatio, type SplitNode } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/SplitLayout",
  component: SplitLayout,
  args: {
    node: { kind: "pane", id: "Preview" } as SplitNode,
    renderPane: (id: string) => id,
    onResize: () => {},
  },
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Controlled split panes with nested horizontal/vertical layouts. Drag separators, use arrow keys (Shift for a larger step), or double click to equalize a split. Pane content and tab docking are supplied by the caller.",
      },
    },
  },
  render: function Example() {
    const [node, setNode] = useState<SplitNode>({
      kind: "split",
      id: "outer",
      axis: "horizontal",
      ratio: 0.5,
      first: { kind: "pane", id: "Left" },
      second: {
        kind: "split",
        id: "inner",
        axis: "vertical",
        ratio: 0.5,
        first: { kind: "pane", id: "Top right" },
        second: { kind: "pane", id: "Bottom right" },
      },
    });
    return (
      <div style={{ height: "70vh", padding: 8 }}>
        <SplitLayout
          node={node}
          onResize={(id, ratio) =>
            setNode((current) => setSplitRatio(current, id, ratio))
          }
          renderPane={(id) => (
            <div
              style={{
                padding: 16,
                height: "100%",
                background: "var(--ui-shade-27)",
              }}
            >
              {id}
            </div>
          )}
        />
      </div>
    );
  },
} satisfies Meta<typeof SplitLayout>;
export default meta;
export const Nested: StoryObj<typeof meta> = {};
