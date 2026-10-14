import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ConfirmationDialog } from "@lesomnus/nox";
import { Button } from "@lesomnus/nox";

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
          "A modal confirmation dialog using shared DialogCard region backgrounds and blur, with DialogSurface dimming and entrance motion, with padded header/body/footer, a distinct rounded translucent body inset from the card sides and minimum-width actions. The body has generous vertical padding, and the footer uses compact padding; buttons have transparent default surfaces and hover backgrounds. Both actions align right, with Confirm immediately before Cancel; focus starts on Cancel and stays inside. Escape or a backdrop click cancels. The footer stays outside the scrollable body. While an action runs, duplicate execution and dismissal are disabled. Failure keeps the dialog open for retry. Centered confirmations keep their trigger under the backdrop; foreground triggers are reserved for adjacent dialogs.",
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

export const BlurredBackground: Story = {
  decorators: [
    (Story) => (
      <div style={{ position: "relative" }}>
        <div
          aria-hidden="true"
          style={{
            position: "fixed",
            inset: 0,
            pointerEvents: "none",
            background:
              "repeating-linear-gradient(90deg, #141414 0 8px, #888 8px 16px)",
            opacity: 0.7,
          }}
        />
        <div style={{ position: "relative" }}>
          <Story />
        </div>
      </div>
    ),
  ],
};

export const TriggerForeground: Story = {
  decorators: [
    (Story) => (
      <div
        style={{
          position: "fixed",
          right: "calc(4 * var(--ui-space))",
          bottom: "calc(4 * var(--ui-space))",
        }}
      >
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        story:
          "The trigger stays visible above the dimmed page in its original position, as a disabled visual copy. It cannot act or receive keyboard focus; the dialog surface stays in front.",
      },
    },
  },
};
