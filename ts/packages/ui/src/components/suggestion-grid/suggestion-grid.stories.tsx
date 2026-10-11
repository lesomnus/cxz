import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SuggestionGrid, moveGridSelection } from "@lesomnus/cxz-ui";

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
