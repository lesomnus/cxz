import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueuedMessage } from "./queued-message";
function QueuePreview() {
  const [queued, setQueued] = useState(
    "Review the test failures when this turn finishes.",
  );
  const [draft, setDraft] = useState("");
  return (
    <div style={{ width: "min(600px, 100vw)" }}>
      <QueuedMessage
        text={queued}
        busy={false}
        canRecall={!draft}
        recall={() => {
          setDraft(queued);
          setQueued("");
        }}
        cancel={() => setQueued("")}
      />
      <textarea
        aria-label="Draft"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />
    </div>
  );
}
const meta = {
  title: "Conversation/QueuedMessage",
  component: QueuePreview,
} satisfies Meta<typeof QueuePreview>;
export default meta;
export const Waiting: StoryObj<typeof meta> = {};
