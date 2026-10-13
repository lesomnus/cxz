import { create } from "@bufbuild/protobuf";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AuxKind } from "#gen/cxz/project_svc_pb";
import { AuxStateSchema, AuxSchema } from "#gen/cxz/session_svc_pb";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import {
  fileEvents,
  activityStackEvents,
  responseEvent,
  storyEvent,
  toolEvents,
  questionEvent,
} from "#src/storybook/fixtures.ts";

const meta = {
  title: "Conversation/EventCards",
  component: ComponentPreview,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Conversation responses and errors stay in the transcript. Tool executions collapse into one summary per turn; View details opens an in-conversation drawer. Select a task to read its Input/Output/Result tabs.",
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
const question = questionEvent("steps");
question.requestId = "question-history";
const questionAnswer = storyEvent(6, "approval_resolved", "allowed", {
  state: "allowed",
  answers: {
    layout: { selected: ["Compact"] },
    notes: {
      selected: [],
      other:
        "Keep the existing toolbar.\n\n```typescript\nconst compact = true;\n```",
    },
    timing: { selected: ["Now"] },
  },
});
questionAnswer.requestId = question.requestId;
export const AnsweredQuestion: Story = {
  args: { events: [question, questionAnswer] },
};
const canceledQuestion = storyEvent(6, "approval_resolved", "denied");
canceledQuestion.requestId = question.requestId;
export const CanceledQuestion: Story = {
  args: { events: [question, canceledQuestion] },
};
export const FileChanges: Story = { args: { events: fileEvents() } };
export const TurnTasks: Story = {
  args: { events: activityStackEvents() },
  parameters: {
    docs: {
      description: {
        story:
          "A single summary represents all work in this turn. Its drawer stays within the conversation, with a virtualized task list and on-demand details.",
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
export const ModelAtCapacity: Story = {
  args: {
    events: [
      storyEvent(1, "diagnostic", "Codex error", {
        error: {
          message:
            "Selected model is at capacity. Please try a different model.",
          codexErrorInfo: "serverOverloaded",
          additionalDetails: null,
          misalignment: null,
        },
        willRetry: false,
      }),
    ],
  },
};
export const FailedTool: Story = { args: { events: toolEvents("failed") } };

export const EarlierError: Story = {
  args: {
    events: [
      ...ModelAtCapacity.args!.events!,
      ...toolEvents(),
      responseEvent(6),
    ],
  },
};

const legacyError = storyEvent(1, "diagnostic", "Codex error");
legacyError.payload = new Uint8Array();
export const LegacyErrorHistory: Story = {
  args: {
    events: [legacyError, ...toolEvents()],
    loadDetails: async () => ModelAtCapacity.args!.events!,
  },
  parameters: {
    docs: {
      description: {
        story:
          "Older Transcript servers omit the diagnostic JSON. A visible error card reads native details to replace the generic label with error.message. Error cards stay upright and unfolded even when later tools arrive.",
      },
    },
  },
};

type AuxRun = Parameters<typeof create<typeof AuxSchema>>[1];
const summaryState = (over?: AuxRun, text = "") =>
  create(AuxStateSchema, {
    summaries: text ? [{ runId: "storybook", turn: 2n, text }] : [],
    ...(over
      ? {
          current: create(AuxSchema, { runId: "storybook", turn: 2n, ...over }),
        }
      : {}),
  });

export const TurnSummary: Story = {
  args: {
    events: [responseEvent()],
    aux: summaryState(
      undefined,
      "Reviewed the conversation components and agreed to keep the cards compact. **No code changed yet.**",
    ),
  },
  parameters: {
    docs: {
      description: {
        story:
          "A summary is one auxiliary task's result: what a model wrote about the turn, not what the agent answered. It is labelled and set apart below the response footer, because an unlabelled paragraph in a conversation reads as something somebody said. The server names a turn by its turn_end sequence; this view attaches the summary to the last row at or before it, which is the final response in both live and projected history.",
      },
    },
  },
};
export const TurnSummaryPending: Story = {
  args: {
    events: [responseEvent()],
    aux: summaryState({ state: "running", kinds: [AuxKind.SUMMARY] }),
  },
  parameters: {
    docs: {
      description: {
        story:
          "A summary that was asked for and has not arrived says so. Showing nothing would be indistinguishable from a session that generates no summaries.",
      },
    },
  },
};
export const TurnSummaryFailed: Story = {
  args: {
    events: [responseEvent()],
    aux: summaryState({
      state: "failed",
      kinds: [AuxKind.SUMMARY],
      message: "Summary account is not configured.",
    }),
  },
  parameters: {
    docs: {
      description: {
        story:
          "A failure takes the summary's place rather than being hidden: the reason is what a reader can act on.",
      },
    },
  },
};
