import type { Meta, StoryObj } from "@storybook/react-vite";
import { UsageInfo } from "./usage-info";

const meta = {
  title: "Conversation/Usage",
  component: UsageInfo,
  decorators: [
    (Story) => (
      <div
        style={{
          display: "flex",
          justifyContent: "flex-end",
          padding: 24,
          width: 520,
          maxWidth: "100vw",
        }}
      >
        <Story />
      </div>
    ),
  ],
  args: {
    agent: "codex",
    info: {
      model: "",
      effort: "",
      remaining: 100,
      quotaLabel: "codex · 5h",
      reset: Date.now() + 5 * 3600_000,
      contextUsed: 48000,
      contextWindow: 200000,
    },
  },
} satisfies Meta<typeof UsageInfo>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Full: Story = {};
export const Low: Story = {
  args: {
    agent: "claude",
    info: {
      ...meta.args.info,
      remaining: 1,
      quotaLabel: "five hour",
      reset: Date.now() + 60000,
    },
  },
};
export const Weekly: Story = {
  args: {
    info: {
      ...meta.args.info,
      remaining: 72,
      quotaLabel: "codex · weekly",
      reset: Date.now() + 7 * 24 * 3600_000,
    },
  },
};
export const Unreported: Story = { args: { info: { model: "", effort: "" } } };
