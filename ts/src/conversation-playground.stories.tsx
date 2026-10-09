import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConversationPlayground } from "./storybook/conversation-playground";

const meta = {
  title: "Conversation/Playground",
  component: ConversationPlayground,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "A serverless conversation using the application's composer, virtual transcript, event cards and send animation. Send a message with the arrow or Ctrl+Enter. Paste multiline text to create a chip; double-click a tool card to open its Monaco details. Reset clears the local simulation. Terminal connections are disabled.",
      },
    },
  },
  args: {
    agent: "codex",
    acknowledgementMs: 450,
    responseMs: 1800,
    historyTurns: 1,
  },
  argTypes: {
    agent: { control: "select", options: ["codex", "claude"] },
    acknowledgementMs: {
      control: { type: "range", min: 0, max: 3000, step: 50 },
    },
    responseMs: { control: { type: "range", min: 0, max: 10000, step: 100 } },
  },
} satisfies Meta<typeof ConversationPlayground>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Interactive: Story = {};
export const Claude: Story = { args: { agent: "claude" } };
export const LongConversation: Story = { args: { historyTurns: 90 } };
export const SlowAcknowledgement: Story = { args: { acknowledgementMs: 2400 } };
