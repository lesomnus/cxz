import type { Meta, StoryObj } from "@storybook/react-vite";
import { IdentifierValue } from "./identifier-value";

const meta = {
  title: "Components/IdentifierValue",
  component: IdentifierValue,
  args: { value: "1a2b3c4d-5e6f-7788-9900-aabbccddeeff" },
  parameters: {
    docs: {
      description: {
        component:
          "A stable-width identifier prefix with an in-place full-value overlay on hover/focus (same text origin and baseline) and a copy button that copies the original value.",
      },
    },
  },
} satisfies Meta<typeof IdentifierValue>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Short: Story = { args: { value: "oak" } };
export const Missing: Story = { args: { value: undefined } };
