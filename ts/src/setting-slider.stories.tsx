import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { SettingSlider } from "./setting-slider";

const meta = {
  title: "Components/SettingSlider",
  component: SettingSlider,
  tags: ["autodocs"],
  args: {
    label: "Indentation size",
    value: 4,
    inherited: 3,
    disabled: false,
    onChange: () => {},
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
      <SettingSlider {...args} onChange={(value) => updateArgs({ value })} />
    );
  },
} satisfies Meta<typeof SettingSlider>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Inherited: Story = { args: { value: undefined } };
export const AboveRange: Story = { args: { value: 12 } };
export const Disabled: Story = { args: { disabled: true } };
