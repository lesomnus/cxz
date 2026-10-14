import type { Meta, StoryObj } from "@storybook/react-vite";
import { CompactNumber } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/CompactNumber",
  component: CompactNumber,
  tags: ["autodocs"],
  args: { value: 1 },
  parameters: {
    docs: {
      description: {
        component:
          "A fixed-width numeric display. Compact mode selects a unit that keeps as much information as fits; units are slightly dimmed and padding zeroes are darker. Integer mode omits the unit cell for ordinary counters. Small fractional values use m, µ and n so costs do not disappear as zero. The accessible label retains the original value.",
      },
    },
  },
} satisfies Meta<typeof CompactNumber>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Single: Story = {};
export const Integer: Story = { args: { value: 1, mode: "integer" } };
export const DetailedMillions: Story = { args: { value: 1_234_567 } };
export const Zero: Story = { args: { value: 0 } };
export const Thousands: Story = { args: { value: 1230 } };
export const Millions: Story = { args: { value: 1_230_000 } };
export const SmallCost: Story = {
  args: { value: 0.0001, label: "Cost: $0.0001" },
};
