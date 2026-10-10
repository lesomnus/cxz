import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { ValueMenu } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/ValueMenu",
  component: ValueMenu,
  tags: ["autodocs"],
  args: {
    choose: () => {},
    label: "Effort",
    value: "medium",
    disabled: false,
    options: [
      { value: "default", label: "medium", muted: true },
      { value: "low", label: "low" },
      { value: "medium", label: "medium" },
      { value: "high", label: "high" },
    ],
  },
  render: function Interactive(args) {
    const [, updateArgs] = useArgs();
    return <ValueMenu {...args} choose={(value) => updateArgs({ value })} />;
  },
} satisfies Meta<typeof ValueMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Inherited: Story = { args: { value: "default" } };
export const Disabled: Story = {
  args: { disabled: true, disabledReason: "Settings require an idle session" },
};

export const ComposerValue: Story = {
  args: { variant: "compact", minMenuWidth: 140 },
  decorators: [
    (Story) => (
      <div style={{ fontSize: 11 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span className="meta-label">Effort</span>
          <Story />
        </div>
      </div>
    ),
  ],
};
