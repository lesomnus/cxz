import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConversationLoading } from "./conversation-loading";

const meta = {
  title: "Conversation/Loading",
  component: ConversationLoading,
  decorators: [
    (Story) => (
      <div
        style={{ position: "relative", width: "min(600px, 90vw)", height: 320 }}
      >
        <Story />
      </div>
    ),
  ],
  args: { stage: "session" },
} satisfies Meta<typeof ConversationLoading>;
export default meta;
export const Session: StoryObj<typeof meta> = {};
export const History: StoryObj<typeof meta> = { args: { stage: "history" } };
export const Retrying: StoryObj<typeof meta> = {
  args: { stage: "retrying", error: "[unavailable] Connection interrupted" },
};
