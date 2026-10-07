import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { ValueMenu } from "./value-menu";

const meta = {
  title: "Components/ValueMenu",
  component: ValueMenu,
  tags: ["autodocs"],
  args: {
    choose: () => {},
    label: "추론 강도",
    value: "medium",
    disabled: false,
    options: [
      { value: "default", label: "기본값", muted: true },
      { value: "low", label: "낮음" },
      { value: "medium", label: "보통" },
      { value: "high", label: "높음" },
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
  args: { disabled: true, disabledReason: "실행 중에는 변경할 수 없습니다." },
};
