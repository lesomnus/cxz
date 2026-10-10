import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QuestionCard } from "./question-card";
import { Button } from "./button";
import {
  ComponentPreview,
  previewPastes,
} from "./storybook/conversation-preview";
import {
  questionEvent,
  toolEvents,
  type QuestionExample,
} from "./storybook/fixtures";

function QuestionPreview({
  busy = false,
  example = "choice",
}: {
  busy?: boolean;
  example?: QuestionExample;
}) {
  const [answer, setAnswer] = useState<string>();
  const pastes = useRef(previewPastes());
  return (
    <ComponentPreview events={toolEvents()}>
      {answer === undefined ? (
        <QuestionCard
          e={questionEvent(example)}
          agent={example === "previews" ? "claude" : "codex"}
          pastes={pastes.current}
          busy={busy}
          reply={async (_, allow, answers) =>
            setAnswer(allow ? answers || "Allowed" : "Denied")
          }
        />
      ) : (
        <div className="floating-card">
          <div className="card-body">
            <p role="status">{answer}</p>
            <Button onClick={() => setAnswer(undefined)}>Ask again</Button>
          </div>
        </div>
      )}
    </ComponentPreview>
  );
}
const meta = {
  title: "Conversation/QuestionCard",
  component: QuestionPreview,
  tags: ["autodocs"],
  args: { busy: false, example: "choice" },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "A persistent question with radio cards, Markdown option previews and the same multiline editor for free-text and Other answers. Multiple questions use keyboard-accessible tabs that preserve selections, drafts and undo. Codex asynchronous questions use the same reply keys as the TUI. Select an option or write an answer and submit. Open Request details to preview the covered-card hierarchy. Only an explicit answer or Deny dismisses the question.",
      },
    },
  },
} satisfies Meta<typeof QuestionPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Question: Story = {};
export const Busy: Story = { args: { busy: true } };

export const FreeText: Story = { args: { example: "free-text" } };
export const Steps: Story = { args: { example: "steps" } };
export const AsyncSteps: Story = { args: { example: "async-steps" } };
export const OptionPreviews: Story = { args: { example: "previews" } };
