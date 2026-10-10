import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { Switch } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/Switch",
  component: Switch,
  tags: ["autodocs"],
  args: {
    label: "Copy on selection",
    checked: true,
    disabled: false,
    onChange: () => {},
  },
  render: function Interactive(args) {
    const [, updateArgs] = useArgs();
    return <Switch {...args} onChange={(checked) => updateArgs({ checked })} />;
  },
} satisfies Meta<typeof Switch>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Off: Story = { args: { checked: false } };
export const Disabled: Story = { args: { disabled: true } };
