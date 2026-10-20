import type { Meta, StoryObj } from "@storybook/react-vite";
import { FieldLabel } from "./field-label";

const meta = {
  title: "Components/FieldLabel",
  component: FieldLabel,
  tags: ["autodocs"],
  args: {
    label: "Provisioning",
    description:
      "Status and current step of preparing the project container and workspace.",
  },
} satisfies Meta<typeof FieldLabel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const WithHelp: Story = {};
export const RuntimeID: Story = {
  args: {
    label: "Runtime ID",
    description:
      "Project identifier used in API calls and links. Stored as runtime_id, separately from the database UUID and container ID.",
  },
};
export const Plain: Story = { args: { label: "Name", description: undefined } };
