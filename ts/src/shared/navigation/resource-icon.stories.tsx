import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResourceIcon } from "./resource-icon";

const meta = {
  title: "Navigation/ResourceIcon",
  component: ResourceIcon,
  args: { kind: "sessions", unreadCount: 0 },
  decorators: [
    (Story) => (
      <div
        className="resource-link"
        style={{ display: "grid", placeItems: "center" }}
      >
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ResourceIcon>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
export const Unread: StoryObj<typeof meta> = { args: { unreadCount: 1 } };
export const MultipleUnread: StoryObj<typeof meta> = {
  args: { unreadCount: 12 },
};
export const ManyUnread: StoryObj<typeof meta> = { args: { unreadCount: 123 } };

export const UnreadError: StoryObj<typeof meta> = {
  args: { unreadCount: 3, error: true },
};
