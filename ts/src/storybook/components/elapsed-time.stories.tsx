import type { Meta, StoryObj } from "@storybook/react-vite";
import { ElapsedTime } from "@lesomnus/nox";

const meta = {
  title: "Components/ElapsedTime",
  component: ElapsedTime,
  tags: ["autodocs"],
  args: { label: "Response elapsed time" },
} satisfies Meta<typeof ElapsedTime>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Idle: Story = {};
export const Completed: Story = {
  args: { durationMs: 42_000, label: "Completed response duration" },
};
export const Running: Story = { args: { startedAt: Date.now() - 42_000 } };
export const LongRunning: Story = {
  args: { startedAt: Date.now() - 3_900_000 },
};
