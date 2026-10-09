import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QuestionCard } from "./question-card";
import { Button } from "./button";
import {
  ComponentPreview,
  previewPastes,
} from "./storybook/conversation-preview";
import { questionEvent, toolEvents } from "./storybook/fixtures";

function QuestionPreview({ busy = false }: { busy?: boolean }) {
  const [answer, setAnswer] = useState<string>();
  const pastes = useRef(previewPastes());
  return (
    <ComponentPreview events={toolEvents()}>
      {answer === undefined ? (
        <QuestionCard
          e={questionEvent()}
          agent="codex"
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
  args: { busy: false },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "A persistent question with radio cards and the same multiline editor for Other answers. Select an option or write an answer and submit. Open Request details to preview the covered-card hierarchy. Only an explicit answer or Deny dismisses the question.",
      },
    },
  },
} satisfies Meta<typeof QuestionPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Question: Story = {};
export const Busy: Story = { args: { busy: true } };
