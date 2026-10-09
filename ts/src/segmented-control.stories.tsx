import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { SegmentedControl } from "./segmented-control";

const meta = {
  title: "Components/SegmentedControl",
  component: SegmentedControl,
  tags: ["autodocs"],
  args: {
    onChange: () => {},
    label: "Theme",
    value: "dark",
    disabled: false,
    options: [
      { value: "system", label: "System", muted: true },
      { value: "light", label: "Light" },
      { value: "dark", label: "Dark" },
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

export const TabInput: Story = {
  args: {
    label: "Tab input",
    value: "spaces",
    options: [
      { value: "default", label: "Spaces", muted: true },
      { value: "spaces", label: "Spaces" },
      { value: "tab", label: "Tab" },
    ],
  },
};
