import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PanelItem } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/PanelItem",
  component: PanelItem,
  tags: ["autodocs"],
} satisfies Meta<typeof PanelItem>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Selection: Story = {
  render: function Selection() {
    const [selected, select] = useState("General");
    return (
      <nav aria-label="Settings topics" style={{ width: "12rem" }}>
        {["General", "Editor", "Terminal"].map((label) => (
          <PanelItem
            key={label}
            aria-pressed={selected === label}
            onClick={() => select(label)}
          >
            {label}
          </PanelItem>
        ))}
      </nav>
    );
  },
};
