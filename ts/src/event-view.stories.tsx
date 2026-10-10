import type { Meta, StoryObj } from "@storybook/react-vite";
import { ComponentPreview } from "./storybook/conversation-preview";
import {
  fileEvents,
  activityStackEvents,
  responseEvent,
  storyEvent,
  toolEvents,
} from "./storybook/fixtures";

const meta = {
  title: "Conversation/EventCards",
  component: ComponentPreview,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Production event cards, including response metadata, copy-on-hover and tool detail overlays. Double-click a tool card to open its Input/Output/Result editor tabs; click elsewhere in the transcript to dismiss.",
      },
    },
  },
  args: { agent: "codex", events: [responseEvent()] },
  argTypes: { events: { control: false } },
} satisfies Meta<typeof ComponentPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const FinalResponse: Story = {};
export const ClaudeResponse: Story = {
  args: { agent: "claude", events: [responseEvent(2, "claude")] },
};
export const UserInput: Story = {
  args: {
    events: [
      storyEvent(
        1,
        "input",
        "Please keep **conversation cards** compact.\n\n- Use the shared controls\n- Include `model · effort` metadata",
      ),
    ],
  },
};
const commentary = responseEvent(
  2,
  "codex",
  "I’m checking the current components before making changes.",
);
commentary.response!.phase = "commentary";
commentary.response!.completionJson = new Uint8Array();
export const Commentary: Story = { args: { events: [commentary] } };
export const CompletedTool: Story = { args: { events: toolEvents() } };
export const WorkingTool: Story = { args: { events: toolEvents("working") } };
export const FileChanges: Story = { args: { events: fileEvents() } };
export const FoldedActivityStack: Story = {
  args: { events: activityStackEvents() },
  parameters: {
    docs: {
      description: {
        story:
          "Earlier tool cards keep one line on a tilted translucent face; the newest activity remains unfolded. Hover or keyboard focus straightens and unfolds an earlier face above its neighbors without moving the transcript; double-click opens the existing detail editor.",
      },
    },
  },
};
export const Diagnostic: Story = {
  args: {
    events: [
      storyEvent(1, "diagnostic", "The workspace connection was interrupted.", {
        retryable: true,
      }),
    ],
  },
};
