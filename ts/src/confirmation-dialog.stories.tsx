import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ConfirmationDialog } from "./confirmation-dialog";
import { Button } from "./button";

const meta = {
  title: "Components/ConfirmationDialog",
  component: ConfirmationDialog,
  tags: ["autodocs"],
  args: {
    title: "Stop session",
    confirmLabel: "Stop",
    children: (
      <p>Active work stops. Conversation history and login are retained.</p>
    ),
    execute: async () => true,
    close: () => {},
  },
  parameters: {
    docs: {
      description: {
        component:
          "A modal confirmation dialog using the shared card surface, with padded header/body/footer, a distinct rounded body flush with the card sides and minimum-width actions. The body has generous vertical padding, and the footer uses compact padding; buttons have transparent default surfaces and hover backgrounds. Both actions align right, with Confirm immediately before Cancel; focus starts on Cancel and stays inside. Escape or a backdrop click cancels. The footer stays outside the scrollable body. While an action runs, duplicate execution and dismissal are disabled. Failure keeps the dialog open for retry.",
      },
    },
  },
  render: function Interactive(args) {
    const [open, setOpen] = useState(false);
    const [complete, setComplete] = useState(false);
    return (
      <>
        <Button
          onClick={() => {
            setComplete(false);
            setOpen(true);
          }}
        >
          Open confirmation
        </Button>
        {complete && <p role="status">Action completed</p>}
        {open && (
          <ConfirmationDialog
            {...args}
            close={() => setOpen(false)}
            execute={async () => {
              const result = await args.execute();
              if (result) setComplete(true);
              return result;
            }}
          />
        )}
      </>
    );
  },
} satisfies Meta<typeof ConfirmationDialog>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Purge: Story = {
  args: {
    title: "Purge session",
    confirmLabel: "Purge permanently",
    children: (
      <p>
        Permanently deletes this session and its stored data. This cannot be
        undone.
      </p>
    ),
  },
};
export const Pending: Story = {
  args: {
    execute: () =>
      new Promise<boolean>((resolve) => setTimeout(() => resolve(true), 1500)),
  },
};
export const Failure: Story = { args: { execute: async () => false } };
export const LongContent: Story = {
  args: {
    title: "Purge session",
    confirmLabel: "Purge permanently",
    children: (
      <>
        <p>Permanently deletes this session and its stored data.</p>
        {Array.from({ length: 16 }, (_, index) => (
          <p key={index}>
            Resource {index + 1}: saved conversation history and temporary
            session artifacts will be removed. Project workspace files are
            retained.
          </p>
        ))}
      </>
    ),
  },
};
