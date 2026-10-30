import { useEffect, useRef, useState } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { useVisibleApprovals } from "#src/features/session/model/use-visible-approvals.ts";
import { pendingAfter } from "#src/features/session/model/journal.ts";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QuestionCard } from "./question-card";
import { Button } from "@lesomnus/nox";
import type { UploadProgress } from "#src/features/session/composer/composer-upload.ts";
import {
  ComponentPreview,
  previewPastes,
} from "#src/storybook/conversation-preview.tsx";
import {
  questionEvent,
  toolEvents,
  responseEvent,
  storyEvent,
  type QuestionExample,
} from "#src/storybook/fixtures.ts";

function QuestionPreview({
  busy = false,
  example = "choice",
  uploadMode,
  secretFailure = false,
  uploadDelay = 1000,
}: {
  busy?: boolean;
  example?: QuestionExample;
  uploadMode?: "ready" | "retry";
  secretFailure?: boolean;
  uploadDelay?: number;
}) {
  const [answer, setAnswer] = useState<string>();
  const [draft, setDraft] = useState("");
  const pastes = useRef(previewPastes());
  const [uploads, setUploads] = useState(0);
  const [writes, setWrites] = useState(0);
  const [deletes, setDeletes] = useState(0);
  const values = useRef(new Map<string, Uint8Array>());
  const attempts = useRef(new Set<string>());
  useEffect(() => () => values.current.forEach((value) => value.fill(0)), []);
  const transfer = async (
    size: number,
    signal: AbortSignal,
    progress?: UploadProgress,
  ) => {
    for (const ratio of [0, 0.25, 0.5, 0.75, 1]) {
      signal.throwIfAborted();
      progress?.(Math.ceil(size * ratio), size);
      await new Promise((resolve) => setTimeout(resolve, uploadDelay / 5));
    }
    signal.throwIfAborted();
  };
  return (
    <ComponentPreview
      value={draft}
      onChange={setDraft}
      events={
        example === "long"
          ? Array.from({ length: 25 }, (_, index) =>
              responseEvent(
                index + 1,
                "codex",
                `History response ${index + 1}\n\nRead the conversation while the question stays available.\n\nAdditional context for this response.`,
              ),
            )
          : toolEvents()
      }
    >
      {uploadMode && (
        <div aria-label="Upload activity">
          <output aria-label="File uploads">{uploads}</output>
          <output aria-label="Secret writes">{writes}</output>
          <output aria-label="Deleted secret files">{deletes}</output>
        </div>
      )}
      {answer === undefined ? (
        <QuestionCard
          e={questionEvent(example)}
          agent={example === "previews" ? "claude" : "codex"}
          pastes={pastes.current}
          upload={
            uploadMode
              ? async (file, signal, progress) => {
                  setUploads((count) => count + 1);
                  await transfer(file.size, signal, progress);
                  if (
                    uploadMode === "retry" &&
                    !attempts.current.has(file.name)
                  ) {
                    attempts.current.add(file.name);
                    throw new Error(
                      "Simulated upload failure. Retry from the chip.",
                    );
                  }
                  return `/cxz/assets/storybook/upload/${file.name}`;
                }
              : undefined
          }
          secretFiles={
            uploadMode
              ? {
                  session: "storybook",
                  run: "run",
                  put: async (bytes, signal, progress) => {
                    setWrites((count) => count + 1);
                    await transfer(bytes.length, signal, progress);
                    if (secretFailure)
                      throw new Error("Simulated secret failure");
                    const path = `/cxz/secrets/${crypto.randomUUID()}/value`;
                    values.current.set(path, bytes.slice());
                    return path;
                  },
                  read: async (path, signal) => {
                    signal.throwIfAborted();
                    const value = values.current.get(path);
                    if (!value) throw new Error("Secret expired");
                    return value.slice();
                  },
                  remove: async (path) => {
                    values.current.get(path)?.fill(0);
                    values.current.delete(path);
                    setDeletes((count) => count + 1);
                  },
                }
              : undefined
          }
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
  args: { busy: false, example: "choice", uploadDelay: 1000 },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "A persistent question with Markdown question text and option previews, radio cards and the same multiline editor for free-text and Other answers. Multiple questions use keyboard-accessible tabs and Nox SlidingPages transitions that preserve selections, drafts and undo. The viewport reserves the tallest question’s height while its body slides; the header and footer stay fixed. Codex asynchronous questions use the same reply keys as the TUI. Select an option or write an answer and submit. The whole card has a bordered dark translucent blurred surface. Its borderless body reaches both sides, shares the card radius and contrasts with the darker choices. Question text has reading padding, and selected Other answers have an outline. Numbered tabs and icon slots stay aligned as completion changes. Only the option list scrolls; background-colored edge fades mark hidden choices and gently extend with outgoing scroll motion. Their height and opacity track the remaining distance, fading before the boundary and clearing immediately at the end. Other and the Cancel/Submit/Next footer stay visible. All steps share the tallest question’s size within the conversation height limit. Scroll the LongQuestions transcript to tuck the card behind the composer, then approach it to reveal it. Use the top-right expansion control to fill the available conversation height; collapse it with the same control or either conversation margin without losing answers. Cancel has an outlined transparent surface and first turns dark red; activate it again to cancel, or leave the button to reset confirmation. Ctrl+Enter in Answer/Other advances to Next while some answers are missing; when all answers are ready it focuses Submit without sending. Only explicit Submit or confirmed Cancel dismisses the question.",
      },
    },
  },
} satisfies Meta<typeof QuestionPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Question: Story = {};
export const Busy: Story = { args: { busy: true } };

export const FreeText: Story = { args: { example: "free-text" } };
export const MarkdownBody: Story = { args: { example: "markdown" } };
export const Steps: Story = { args: { example: "steps" } };
export const AsyncSteps: Story = { args: { example: "async-steps" } };
export const OptionPreviews: Story = { args: { example: "previews" } };

export const Attachments: Story = {
  args: { example: "free-text", uploadMode: "ready" },
  parameters: {
    docs: {
      description: {
        story:
          "Question answers use the conversation editor's file/folder drop, pasted file uploads, Text/File paste modes, whole-value secret detection and chip previews. Progress blocks Submit until every referenced upload is acknowledged. This preview simulates files in memory without contacting a server.",
      },
    },
  },
};
export const FailedUpload: Story = {
  args: { example: "free-text", uploadMode: "retry" },
};
export const FailedSecret: Story = {
  args: { example: "free-text", uploadMode: "ready", secretFailure: true },
};

export const LongQuestions: Story = { args: { example: "long" } };

export const LiveApprovals: Story = {
  render: function ApprovalStream() {
    const [pending, setPending] = useState<SessionEvent[]>([]);
    const visible = useVisibleApprovals(pending);
    const pastes = useRef(previewPastes());
    const sequence = useRef(100);
    const timers = useRef<ReturnType<typeof setTimeout>[]>([]);
    const cancelTimers = () => {
      timers.current.forEach(clearTimeout);
      timers.current = [];
    };
    useEffect(() => cancelTimers, []);
    const request = () => {
      const seq = ++sequence.current;
      const event = storyEvent(
        seq,
        "approval",
        "item/commandExecution/requestApproval",
        {
          params: { itemId: `tool-${seq}`, command: "git status --short" },
        },
      );
      event.requestId = `request-${seq}`;
      return event;
    };
    const receive = (event: SessionEvent) =>
      setPending((old) => pendingAfter(old, [event]));
    const resolve = (event: SessionEvent) => {
      const reply = storyEvent(
        ++sequence.current,
        "approval_resolved",
        "allowed",
      );
      reply.requestId = event.requestId;
      receive(reply);
    };
    const automatic = (remaining: number) => {
      const event = request();
      receive(event);
      timers.current.push(setTimeout(() => resolve(event), 10));
      if (remaining > 1)
        timers.current.push(setTimeout(() => automatic(remaining - 1), 50));
    };
    return (
      <>
        <div style={{ display: "flex", gap: 4 }}>
          <Button
            onClick={() => {
              cancelTimers();
              automatic(30);
            }}
          >
            Simulate automatic approvals
          </Button>
          <Button
            onClick={() => {
              cancelTimers();
              setPending([request()]);
            }}
          >
            Request approval
          </Button>
          <Button
            onClick={() => {
              cancelTimers();
              setPending([questionEvent()]);
            }}
          >
            Ask question
          </Button>
          <Button
            onClick={() => {
              cancelTimers();
              setPending([]);
            }}
          >
            Resolve request
          </Button>
        </div>
        <ComponentPreview working>
          {visible.map((event) => (
            <QuestionCard
              key={`${event.runId}:${event.requestId}:${event.seq}`}
              e={event}
              agent="codex"
              pastes={pastes.current}
              busy={false}
              reply={async () => setPending([])}
            />
          ))}
        </ComponentPreview>
      </>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Automatic native approval/resolution pairs use the real pending reducer and display gate, without flashing cards. Permission requests that remain unresolved become visible; human questions appear immediately.",
      },
    },
  },
};
