import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import { ConversationPlayground } from "./conversation-playground";

const presets = [
  { label: "Near instant · 0 ms", delay: 0 },
  { label: "Moderate · 1.5 s", delay: 1500 },
  { label: "High · 12 s", delay: 12000 },
];

const meta = {
  title: "Conversation/Send latency",
  component: ConversationPlayground,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Uses the production send animation with a simulated request acknowledgement. Send the prefilled draft to watch the initial tug, sustained tension and immediate upward departure on success. High latency also exercises waiting beyond 10 seconds. Reset preview restores the draft for replay. Response delay is separate from acknowledgement delay. Reduced-motion preferences are respected.",
      },
    },
  },
  args: {
    acknowledgementMs: 1500,
    responseMs: 300,
    historyTurns: 0,
    initialDraft: "Watch this message pull down while waiting, then lift off.",
  },
  argTypes: {
    acknowledgementMs: {
      control: { type: "range", min: 0, max: 30000, step: 100 },
    },
  },
} satisfies Meta<typeof ConversationPlayground>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Compare: Story = {
  render: function Comparison(args) {
    const [selected, select] = useState(1);
    return (
      <>
        <nav
          aria-label="Acknowledgement latency"
          style={{ display: "flex", flexWrap: "wrap", gap: 8, padding: 16 }}
        >
          {presets.map((preset, index) => (
            <Button
              key={preset.delay}
              aria-pressed={selected === index}
              onClick={() => select(index)}
            >
              {preset.label}
            </Button>
          ))}
        </nav>
        <ConversationPlayground
          {...args}
          key={selected}
          acknowledgementMs={presets[selected].delay}
        />
      </>
    );
  },
};
export const NearInstant: Story = { args: { acknowledgementMs: 0 } };
export const Moderate: Story = { args: { acknowledgementMs: 1500 } };
export const High: Story = { args: { acknowledgementMs: 12000 } };
