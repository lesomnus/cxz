import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/Button",
  component: Button,
  tags: ["autodocs"],
  args: { children: "New session", disabled: false, type: "button" },
} satisfies Meta<typeof Button>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Disabled: Story = { args: { disabled: true } };
export const LongLabel: Story = {
  args: { children: "Save project settings" },
};

export const ToolbarIcon: Story = {
  args: {
    variant: "toolbar",
    "aria-label": "Send",
    children: (
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        aria-hidden="true"
      >
        <path d="M12 19V5m-6 6 6-6 6 6" />
      </svg>
    ),
  },
};
