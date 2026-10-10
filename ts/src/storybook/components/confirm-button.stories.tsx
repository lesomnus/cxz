import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConfirmButton } from "@lesomnus/cxz-ui";
import { Button } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/ConfirmButton",
  component: ConfirmButton,
  tags: ["autodocs"],
  args: {
    children: "Cancel",
    confirmationLabel: "Confirm cancel",
    type: "button",
    onConfirm: () => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "The first activation arms a dark red confirmation state; a second activation performs the action. Leaving with the mouse, moving keyboard focus away or pressing Escape clears confirmation. Touch users can tap twice. The label keeps its size and ordinary bounded press feedback.",
      },
    },
  },
  render: function Preview(args) {
    const [count, setCount] = useState(0);
    return (
      <div>
        <div className="buttons">
          <ConfirmButton
            {...args}
            onConfirm={() => setCount((old) => old + 1)}
          />
          <Button>Another action</Button>
        </div>
        <p role="status">Confirmed: {count}</p>
      </div>
    );
  },
} satisfies Meta<typeof ConfirmButton>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Disabled: Story = { args: { disabled: true } };
