import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentBrand } from "./agent-brand";

const meta = {
  title: "Components/AgentBrand",
  component: AgentBrand,
  tags: ["autodocs"],
  args: { agent: "codex" },
} satisfies Meta<typeof AgentBrand>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Codex: Story = {};
export const Claude: Story = { args: { agent: "claude" } };
export const Custom: Story = { args: { agent: "custom" } };
