import type { Meta, StoryObj } from "@storybook/react-vite";
import { DateTime } from "./date-time";

const meta = {
  title: "Components/DateTime",
  component: DateTime,
  tags: ["autodocs"],
  args: { value: Date.UTC(2026, 9, 20, 2, 38, 19) },
} satisfies Meta<typeof DateTime>;
export default meta;
type Story = StoryObj<typeof meta>;
export const LocalOffset: Story = {};
export const Winter: Story = {
  args: { value: Date.UTC(2026, 0, 20, 2, 38, 19) },
};
export const Missing: Story = { args: { value: undefined } };
