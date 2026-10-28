import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { useState } from "react";
import { Button } from "@lesomnus/nox";
import { SessionDrafts } from "#src/features/session/model/session-drafts.ts";
import { create } from "@bufbuild/protobuf";
import { responseEvent, storyEvent } from "#src/storybook/fixtures.ts";
import { ProjectPathEntrySchema } from "#gen/cxz/project_svc_pb";
import {
  ComponentPreview,
  samplePaste,
} from "#src/storybook/conversation-preview.tsx";

const meta = {
  title: "Conversation/Composer",
  component: ComponentPreview,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The **Composer** is the complete message-writing area, including its toolbar, editor and status bar.\n\n" +
          "- **Toolbar surface** (`.composer-toolbar-surface`): one glow and glass background extending behind the editor by its top corner radius. The editor overlaps it without changing the button row geometry.\n" +
          "- **Composer toolbar** (`.composer-toolbar`): Stop, elapsed time, Session menu (including Terminal with Ctrl+`), Voice input, Latest and Send. Voice input uses the browser's native Web Speech API and browser language, resolving bare languages such as `ko` to a regional speech locale such as `ko-KR`; unsupported browsers show a disabled microphone with a hover hint. Interim speech appears in a live hint and final text is inserted at the editor caret. Click again or press Escape to stop; sending or switching sessions ends recognition. A red microphone dot indicates listening, and an idle listener has no Listening popover. General settings can force local recognition with voice.processLocally; unsupported browsers or missing language packs show an error without cloud fallback. Depending on the browser, recognition may use an online service.\n" +
          "- **Composer editor** (`ComposerEditor`, `.composer-editor`): line numbers, Markdown, code blocks, paste chips and slash-command suggestions. Its input container is `.composer-input`.\n" +
          "- **Composer status bar** (`.composer-meta`): model, effort, usage and context.\n" +
          "- **Composer surface** (`.composer-wrapper`): the box around the toolbar and input; the status bar sits outside this box.\n\n" +
          "These stories show states of the complete `ConversationComposer`, rather than isolated toolbar/editor/status-bar stories. Shared buttons, ActionMenu (separate popup), ValueMenu (current-value overlay) and the timer have their own Components stories. Submission and response creation are demonstrated in Conversation/Playground.",
      },
    },
  },
  args: { value: "", sending: false, working: false },
  render: function Editor(args) {
    const [, updateArgs] = useArgs();
    return (
      <ComponentPreview {...args} onChange={(value) => updateArgs({ value })} />
    );
  },
} satisfies Meta<typeof ComponentPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Empty: Story = {};
export const MenuOverTranscript: Story = {
  args: {
    uploadMode: "ready",
    events: [
      responseEvent(
        2,
        "codex",
        Array.from(
          { length: 24 },
          (_, index) =>
            `Conversation paragraph ${index + 1}. Open the session menu over this actual response to inspect the scroll rail backdrop. Text, numbers 0123456789 and punctuation stay in the transcript's normal paint layers.`,
        ).join("\n\n"),
      ),
    ],
  },
  parameters: {
    docs: {
      description: {
        story:
          "Open the Session menu over a real transcript response. The external rail sits above ordinary conversation text; no synthetic background or extra foreground layer is used.",
      },
    },
  },
};
export const CursorMemory: Story = {
  render: function CursorMemory() {
    const [drafts] = useState(
      () => new SessionDrafts("storybook-cursor-memory", new Map()),
    );
    const [session, select] = useState("one");
    const [value, setValue] = useState(() => drafts.get("one") ?? "");
    return (
      <>
        <nav aria-label="Preview sessions">
          {["one", "two"].map((id) => (
            <Button
              key={id}
              role="tab"
              aria-selected={session === id}
              onClick={() => {
                setValue(drafts.get(id) ?? "");
                select(id);
              }}
            >
              {id}
            </Button>
          ))}
        </nav>
        <ComponentPreview
          key={session}
          value={value}
          autoFocus
          initialSelection={drafts.selection(session)}
          onSelectionChange={(selection) =>
            drafts.setSelection(session, selection)
          }
          onChange={(text) => {
            drafts.set(session, text);
            setValue(text);
          }}
        />
      </>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Switch sessions to restore the draft, caret/selection direction and editor scroll position. Entry focuses the editor. The tab-local cursor record also survives refresh and is stored separately so caret movements do not rewrite pasted file contents.",
      },
    },
  },
};
export const VoiceInput: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "Click the microphone to start native browser dictation. Choose the recognition language in Settings > General > Voice input. The browser language is the initial fallback; an explicit choice persists as voice.language in settings.json. The input shows a softly folded lime/purple waveform driven by local microphone volume, with no language selector in the toolbar. Sending and switching sessions stop both recognition and metering. A metering failure leaves dictation working. Native recognition may use the browser vendor's online service; the waveform itself never records or uploads audio. This preview uses your real browser microphone; Components/AudioWaveform simulates levels without permissions.",
      },
    },
  },
};
export const Markdown: Story = {
  args: {
    value:
      "Review the `session` component:\n- Keep spacing compact\n- Reuse existing controls",
  },
};
export const CodeBlock: Story = {
  args: {
    value:
      "```typescript\nexport function greet(name: string) {\n  return `Hello, ${name}`;\n}\n```",
  },
};
export const PasteChip: Story = {
  args: { value: `Review this code:\n${samplePaste.token}` },
};
export const Commands: Story = {
  args: { value: "/" },
  parameters: {
    docs: {
      description: {
        story:
          "Focus the editor at the end of the first line. Up/Down navigates suggestions; Tab or Right accepts the selected hint.",
      },
    },
  },
};
export const PathHints: Story = {
  args: {
    value: "Inspect `/workspace/` before changing files.",
    lookupPaths: async (directory, signal) => {
      signal.throwIfAborted();
      const entries =
        directory === "/workspace/"
          ? [
              { name: "src", directory: true },
              { name: "README.md", directory: false },
              { name: "bin", directory: true },
              { name: "run.sh", executable: true },
              { name: "channel.pipe", kind: "fifo" },
              { name: "control.sock", kind: "socket" },
              {
                name: "docs-link",
                symlink: true,
                directory: true,
                linkTarget: "src",
              },
              { name: "disk", kind: "block" },
              { name: "console", kind: "character" },
              ...Array.from({ length: 20 }, (_, i) => ({
                name: `example-${i}.txt`,
              })),
            ]
          : directory === "/workspace/src/"
            ? [
                { name: "app.ts", directory: false },
                { name: "main.go", directory: false },
              ]
            : [];
      return entries.map((entry) => create(ProjectPathEntrySchema, entry));
    },
  },
  argTypes: { lookupPaths: { control: false } },
  parameters: {
    docs: {
      description: {
        story:
          "Place the caret inside the backticks after /workspace/. The responsive path grid sits above or below the typing line without covering it. Candidates fan out from the caret with the best fuzzy match nearest and selected by default. All four arrow keys move between physical cells; Tab or Enter accepts, Escape dismisses and Undo restores the previous draft. Names use different colors for directories, executables, symbolic links, pipes, sockets and devices; hover for metadata. Choosing a directory continues completion inside it. This example simulates path listings; the connected sandbox also supports home paths and /download <path> through the existing project APIs.",
      },
    },
  },
};
export const PendingSend: Story = {
  args: { value: "A message awaiting acknowledgement…", sending: true },
  parameters: {
    docs: {
      description: {
        story:
          "The editor has a subtle, static monochrome grain for a matte surface. The read-only editor dims while Send awaits acknowledgement. A subtle Nox InkSparkle sweep repeats across the input until the request settles. Its theme-aware ink stays behind the text and never intercepts input. Reduced motion keeps a static frame.",
      },
    },
  },
};
export const Working: Story = { args: { working: true } };
export const ForkConversation: Story = {
  args: {
    events: [
      storyEvent(1, "input", "Explore two approaches to the same problem."),
      responseEvent(2, "codex", "The current context is ready to branch."),
    ],
  },
  parameters: {
    docs: {
      description: {
        story:
          "Open Session menu → Fork to simulate a new session identity and alias while preserving the conversation, model and effort. Open Details to inspect the new identity. This preview stays local; the connected app uses Session.Fork and navigates to the new session. Active turns and pending questions or queued input disable Fork.",
      },
    },
  },
};
export const StoppedSession: Story = {
  args: { sessionStopped: true },
  parameters: {
    docs: {
      description: {
        story:
          "Open Session menu to resume the simulated stopped session, restart it, inspect session details or review a fake purge plan. These actions affect only this preview; no server or filesystem is used.",
      },
    },
  },
};

export const FileUploads: Story = {
  args: { uploadMode: "slow" },
  render: function UploadPreview(args) {
    const [value, setValue] = useState("");
    return <ComponentPreview {...args} value={value} onChange={setValue} />;
  },
  parameters: {
    docs: {
      description: {
        story:
          "Drop files or folders into the editor, or open Session menu → Upload files / Upload folder. Folders retain their tree in a tar archive. Pasted text has inline Text/File segments and a short hover excerpt. Text sends the original contents and is ready immediately; File sends the uploaded path and waits for acknowledgement. Selection is saved with the draft. Chips use CompactNumber sizes and muted upload percentages. Their reserved control area and logical selections stay aligned with the native textarea through progress, completion, IME, copying and undo. Ordinary file chips still open attachment details. Ctrl+P explicitly opens full source inspection. This preview simulates upload progress and acknowledgement; no server file is written and Send does not start a turn.",
      },
    },
  },
};
export const FailedUpload: Story = {
  ...FileUploads,
  args: { uploadMode: "retry" },
  parameters: {
    docs: {
      description: {
        story:
          "Each filename fails once. Open its chip and choose Retry upload to finish; deleting the chip removes the send blocker. Other draft text and the caret stay editable while uploads are pending.",
      },
    },
  },
};
