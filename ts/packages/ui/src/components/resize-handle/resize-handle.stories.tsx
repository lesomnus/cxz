import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResizeHandle } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/ResizeHandle",
  component: ResizeHandle,
  args: { value: 240, min: 120, max: 400, onChange: () => {} },
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A controlled pane boundary with pointer capture, arrow keys, Home/End limits and optional double-click reset. The host supplies current size and responsive bounds; resizing preserves its mounted content.",
      },
    },
  },
  render: function Example(args) {
    const [width, setWidth] = useState(args.value);
    return (
      <div
        style={{
          display: "grid",
          gridTemplateColumns: `${width}px var(--ui-resize-handle-size) minmax(0, 1fr)`,
          width: "min(700px, 90vw)",
          height: 240,
        }}
      >
        <div
          style={{
            padding: 16,
            background: "var(--ui-shade-27)",
            overflow: "hidden",
          }}
        >
          File explorer
        </div>
        <ResizeHandle
          {...args}
          value={width}
          onChange={setWidth}
          onReset={() => setWidth(args.value)}
        />
        <div
          style={{
            padding: 16,
            background: "var(--ui-shade-20)",
            overflow: "hidden",
          }}
        >
          File contents
        </div>
      </div>
    );
  },
} satisfies Meta<typeof ResizeHandle>;
export default meta;
export const Horizontal: StoryObj<typeof meta> = {};
