import type { Meta, StoryObj } from "@storybook/react-vite";
import { useArgs } from "storybook/preview-api";
import { useState } from "react";
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
          "- **Composer toolbar** (`.composer-toolbar`): Stop, elapsed time, Session menu, Terminal, Latest and Send.\n" +
          "- **Composer editor** (`ComposerEditor`, `.composer-editor`): line numbers, Markdown, code blocks, paste chips and slash-command suggestions. Its input container is `.composer-input`.\n" +
          "- **Composer status bar** (`.composer-meta`): model, effort, usage and context.\n" +
          "- **Composer surface** (`.composer-wrapper`): the box around the toolbar and input; the status bar sits outside this box.\n\n" +
          "These stories show states of the complete `ConversationComposer`, rather than isolated toolbar/editor/status-bar stories. Shared buttons, value menus and the timer have their own Components stories. Submission and response creation are demonstrated in Conversation/Playground.",
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
          "Focus the editor at the end of the first line. Arrow keys navigate suggestions; Right accepts the selected hint.",
      },
    },
  },
};
export const PendingSend: Story = {
  args: { value: "A message awaiting acknowledgement…", sending: true },
};
export const Working: Story = { args: { working: true } };
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
          "Drop files or folders into the editor, or open Session menu → Upload files / Upload folder. Folders retain their tree in a tar archive. Large pastes become text-file chips. Send stays disabled until every attachment finishes. Click a chip to inspect its path or remove it. This preview simulates upload acknowledgement; no server file is written and Send does not start a turn. The connected WASM sandbox exercises the real Upload RPC with in-memory files.",
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
