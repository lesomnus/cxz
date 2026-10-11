import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  SuggestionGrid,
  moveGridSelection,
  moveRankedGridSelection,
} from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/SuggestionGrid",
  component: SuggestionGrid,
  tags: ["autodocs"],
  args: {
    id: "suggestions",
    label: "Candidates",
    items: Array.from({ length: 24 }, (_, i) => ({
      label: `candidate-${i + 1}`,
      color: i % 3 === 0 ? "#9bb9df" : undefined,
    })),
    selected: 0,
    choose: () => {},
    onColumnsChange: () => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "Responsive candidate matrix with a selected cell and configurable content/color. The host keeps keyboard focus, uses moveGridSelection for physical arrow navigation, and handles acceptance. This component has no filesystem or cxz dependencies.",
      },
    },
  },
  render: function Preview(args) {
    const [selected, setSelected] = useState(0),
      [columns, setColumns] = useState(1);
    return (
      <div
        style={{
          width: "min(600px, 85vw)",
          fontFamily: "monospace",
          background: "var(--ui-shade-16)",
        }}
      >
        <input
          aria-label="Query"
          placeholder="Keep focus here; use arrow keys"
          aria-controls={args.id}
          aria-activedescendant={`${args.id}-${selected}`}
          onKeyDown={(event) => {
            if (event.key.startsWith("Arrow")) {
              event.preventDefault();
              setSelected(
                moveGridSelection(
                  selected,
                  args.items.length,
                  columns,
                  event.key,
                ),
              );
            }
          }}
        />
        <SuggestionGrid
          {...args}
          selected={selected}
          onColumnsChange={setColumns}
          choose={setSelected}
        />
      </div>
    );
  },
} satisfies Meta<typeof SuggestionGrid>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Candidates: Story = {};
export const Empty: Story = {
  args: { items: [], message: "No matching candidates" },
};

export const Radial: Story = {
  args: { anchor: { x: 300, edge: "bottom", distance: 12 } },
  parameters: {
    docs: {
      description: {
        story:
          "Items are relevance-ranked and fan out from the caret anchor at the bottom. Rank zero is selected initially; placement adapts to the available columns.",
      },
    },
  },
  render: function RadialPreview(args) {
    const [selected, setSelected] = useState(0),
      [layout, setLayout] = useState({ columns: 1, order: [] as number[] });
    return (
      <div
        style={{
          width: "min(600px,85vw)",
          fontFamily: "monospace",
          background: "var(--ui-shade-16)",
        }}
      >
        <SuggestionGrid
          {...args}
          selected={selected}
          onLayoutChange={setLayout}
          choose={setSelected}
        />
        <input
          aria-label="Query"
          placeholder="The caret anchor is centered on this edge"
          onKeyDown={(event) => {
            if (event.key.startsWith("Arrow")) {
              event.preventDefault();
              setSelected(
                moveRankedGridSelection(
                  selected,
                  layout.order,
                  layout.columns,
                  event.key,
                ),
              );
            }
          }}
        />
      </div>
    );
  },
};
