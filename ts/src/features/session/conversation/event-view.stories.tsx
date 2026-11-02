import { clone, create } from "@bufbuild/protobuf";
import { useState } from "react";
import { Button } from "@lesomnus/nox";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AuxKind } from "#gen/cxz/installation_pb";
import { AuxStateSchema, AuxSchema } from "#gen/cxz/session_svc_pb";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { payload } from "#src/features/session/model/journal.ts";
import {
  fileEvents,
  activityStackEvents,
  responseEvent,
  storyEvent,
  toolEvents,
  loadToolRecords,
  questionEvent,
  mixedConversationEvents,
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
          "Conversation responses and errors stay in the transcript. Work between replies is summarized as a count followed by an icon, opening an in-conversation dialog. Select a task to read its Input/Output/Result tabs. Response metrics also show values before icons and reserve space for their expected ranges.",
      },
    },
  },
  args: { agent: "codex", events: [responseEvent()] },
  argTypes: { events: { control: false } },
} satisfies Meta<typeof ComponentPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const FinalResponse: Story = {};
export const MermaidResponse: Story = {
  args: {
    events: [
      storyEvent(1, "input", "Show how a message reaches the agent."),
      responseEvent(
        2,
        "codex",
        "The manager forwards the message to the project's agent.\n\n```mermaid\nflowchart TD\n  accTitle: Conversation routing\n  Browser --> Manager\n  Manager --> Runtime[Project runtime]\n  Runtime --> Agent\n  Agent --> Response[Conversation response]\n```\n\nThe original diagram definition remains available from the source and copy controls.",
      ),
    ],
  },
};
const longModel = responseEvent();
longModel.response!.model = "provider/very-long-model-identifier-".repeat(8);
export const LongModelName: Story = { args: { events: [longModel] } };
const recentResponse = responseEvent();
recentResponse.timeMs = BigInt(Date.now() - 5 * 60_000);
export const RecentResponse: Story = { args: { events: [recentResponse] } };
const oldResponse = responseEvent();
oldResponse.timeMs = BigInt(Date.now() - 8 * 86_400_000);
export const OlderResponse: Story = { args: { events: [oldResponse] } };
export const MetricRanges: Story = {
  render: function MetricRanges() {
    const [large, setLarge] = useState(false);
    const [base] = useState(() => responseEvent());
    const event = { ...base, response: { ...base.response! } };
    event.response!.completionJson = new TextEncoder().encode(
      JSON.stringify({
        response_seq: String(event.seq),
        duration_ms: large ? 5_999_000 : 500,
        duration_source: "cxz",
        token_scope: "turn",
        metrics: {
          input_tokens: large ? 999_900 : 1,
          output_tokens: large ? 99_900 : 1,
          cache_read_tokens: large ? 999_900 : 1,
          cost_usd: large ? 99.9999 : 0.0001,
          api_duration_ms: large ? 5_999_000 : 500,
          api_turns: large ? 999 : 1,
          tool_calls: large ? 999 : 1,
        },
      }),
    );
    return (
      <>
        <Button onClick={() => setLarge(!large)}>
          {large ? "Use small values" : "Use large values"}
        </Button>
        <ComponentPreview events={[event]} />
      </>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Switch between short and long metric values. The value slots, icons and neighboring metrics retain their positions; values are right-aligned before their icons. Narrow viewports wrap whole metrics without changing each metric's reserved width.",
      },
    },
  },
};
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
// A card's output comes from the records its row was reduced from, fetched
// when the card opens. A story that cannot answer for them shows a card with
// no output, which is a fixture gap rather than a product one.
export const CompletedTool: Story = {
  args: { events: toolEvents(), loadDetails: loadToolRecords() },
};
export const WorkingTool: Story = { args: { events: toolEvents("working") } };
const question = questionEvent("steps");
question.requestId = "question-history";
// A resolved question arrives as one row with its answer folded in. The
// runtime does that folding -- the resolution record is never a row -- so a
// fixture that emitted both would exercise a reduction the app no longer has.
// See cxz#155.
const resolved = (state: string, resolution: Record<string, unknown> = {}) => {
  const row = clone(SessionEventSchema, question);
  row.payload = new TextEncoder().encode(
    JSON.stringify({
      ...payload(question),
      cxz_resolution: { ...resolution, state },
    }),
  );
  return row;
};
const answeredQuestion = resolved("allowed", {
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
export const AnsweredQuestion: Story = {
  args: { events: [answeredQuestion] },
};
const longQuestionHistory = clone(SessionEventSchema, answeredQuestion);
const longQuestionPayload = payload(longQuestionHistory);
longQuestionPayload.params.questions[1].question =
  "Which changes should we keep?\n\n" +
  Array.from(
    { length: 18 },
    (_, index) =>
      `- **Requirement ${index + 1}:** Keep keyboard navigation, readable Markdown and the existing toolbar.`,
  ).join("\n");
longQuestionPayload.cxz_resolution.answers.notes.other =
  "Keep the existing toolbar.\n\n" +
  Array.from(
    { length: 12 },
    (_, index) =>
      `### Change ${index + 1}\n\nPreserve focus and stable conversation layout.\n\n\`\`\`typescript\nconst compact = true;\n\`\`\``,
  ).join("\n\n");
longQuestionHistory.payload = new TextEncoder().encode(
  JSON.stringify(longQuestionPayload),
);
export const LongQuestionHistory: Story = {
  args: {
    events: [
      longQuestionHistory,
      responseEvent(
        6,
        "codex",
        "This response stays in place when you switch question tabs.",
      ),
    ],
  },
  parameters: {
    docs: {
      description: {
        story:
          "Switch between short and overflowing questions and answers. Nox SlidingPages moves each question and its answer together in a fixed viewport; the response card and both content areas keep their heights. Each area scrolls independently using the shared ScrollArea, retaining its position when revisited. Unopened question content is mounted on demand. The following response stays in place.",
      },
    },
  },
};
const beforeQuestion = responseEvent(
  2,
  "codex",
  "Before updating the composer, I will confirm the layout, additional changes and timing.",
);
beforeQuestion.response!.phase = "commentary";
beforeQuestion.response!.completionJson = new Uint8Array();
export const QuestionHistoryInConversation: Story = {
  args: {
    events: [
      storyEvent(
        1,
        "input",
        "Update the composer while keeping the existing toolbar.",
      ),
      beforeQuestion,
      clone(SessionEventSchema, answeredQuestion),
      responseEvent(
        6,
        "codex",
        "Your answers are recorded: **Compact**, keep the existing toolbar, and ship **Now**. I will apply those choices.",
      ),
      storyEvent(
        7,
        "input",
        "Keep that question and its answers in the conversation so I can review the decision later.",
      ),
      responseEvent(
        8,
        "codex",
        "The conversation retains the question and its recorded answers above. Select a question tab to review its recorded answer.",
      ),
    ],
  },
  parameters: {
    docs: {
      description: {
        story:
          "A completed multi-step Question uses the same header and surface as an agent response. Tabs in its body slide between each question and its recorded answer using Nox SlidingPages, including Markdown and code. The card, question and answer areas keep their heights, with internal scrolling for overflow and retained scroll positions. This is history, so no live answer form or Submit action is opened.",
      },
    },
  },
};
export const LongMixedConversation: Story = {
  args: { events: mixedConversationEvents(180) },
  parameters: {
    docs: {
      description: {
        story:
          "A long history mixing many short inputs/replies and resolved question cards. Drag the conversation handle quickly to compare rendering and nested card scrolling.",
      },
    },
  },
};
export const CanceledQuestion: Story = {
  args: { events: [resolved("denied")] },
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
export const FailedTool: Story = {
  args: {
    events: toolEvents("failed"),
    loadDetails: loadToolRecords("failed"),
  },
};

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

const codeComparison =
  'Inline `const message = "hello"` and a fenced code block:\n\n```typescript\n// Keep this comment readable.\nconst message = "hello";\nfunction repeat(count: number) {\n  return message.repeat(count + 1);\n}\n```';
export const CodePaletteComparison: Story = {
  args: {
    events: [
      storyEvent(1, "input", codeComparison),
      responseEvent(2, "codex", codeComparison),
    ],
  },
};
