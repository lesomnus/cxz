import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { SegmentedControl } from "./segmented-control";

const meta = {
  title: "Components/SegmentedControl",
  component: SegmentedControl,
  tags: ["autodocs"],
  args: {
    onChange: () => {},
    label: "테마",
    value: "dark",
    disabled: false,
    options: [
      { value: "system", label: "시스템", muted: true },
      { value: "light", label: "밝게" },
      { value: "dark", label: "어둡게" },
    ],
  },
  decorators: [
    (Story) => (
      <div style={{ width: 280 }}>
        <Story />
      </div>
    ),
  ],
  render: function Interactive(args) {
    const [, updateArgs] = useArgs();
    return (
      <SegmentedControl {...args} onChange={(value) => updateArgs({ value })} />
    );
  },
} satisfies Meta<typeof SegmentedControl>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Disabled: Story = { args: { disabled: true } };
