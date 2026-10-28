import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { Button } from "@lesomnus/nox";
import { PasteChip } from "./paste-chip";
import type { ComposerPaste } from "./composer-pastes";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";

const file: ComposerPaste = {
  token: "[File 12345678 · archive.zip]",
  body: "",
  lines: 0,
  bytes: 2500000,
  attachment: { name: "archive.zip", state: "uploading", progress: 35 },
};
const text: ComposerPaste = {
  token: "[Paste 12345678]",
  body: "Pasted text\n".repeat(400),
  lines: 400,
  bytes: 4800,
  attachment: { name: "paste.txt", state: "uploading", progress: 35 },
};
const meta = {
  title: "Composer/Upload chips",
  component: PasteChip,
  args: { paste: file, onOpen: () => {} },
  parameters: {
    docs: {
      description: {
        component:
          "File uploads and large pastes share Nox FileChip's transparent outlined design with conversation file links. Uploads shimmer until acknowledged. Replay the simulated upload to inspect completion. Respects reduced motion.",
      },
    },
  },
} satisfies Meta<typeof PasteChip>;
export default meta;
type Story = StoryObj<typeof meta>;
export const UploadingFile: Story = {};
export const UploadingPaste: Story = { args: { paste: text } };
export const Ready: Story = {
  args: {
    paste: { ...file, attachment: { ...file.attachment!, state: "ready" } },
  },
};
export const ReadyPaste: Story = {
  args: {
    paste: { ...text, attachment: { ...text.attachment!, state: "ready" } },
  },
};
export const Interactive: Story = {
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        story:
          "Paste multiline or long text, or drop files and folders into the real conversation editor. Chips show upload progress, then their size; click one to preview its contents or remove it. Session menu also provides file and folder pickers. Native paste/drop and the production composer run normally; only upload acknowledgement is simulated, so no files leave the browser.",
      },
    },
  },
  render: function Interactive() {
    const [value, setValue] = useState("");
    return (
      <ComponentPreview
        value={value}
        onChange={setValue}
        uploadMode="slow"
        autoFocus
      />
    );
  },
};
export const UploadTransition: Story = {
  render: function Transition() {
    const [progress, setProgress] = useState(0);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
      const timer = window.setInterval(
        () =>
          setProgress((value) => {
            const next = Math.min(100, value + 5);
            if (next === 100) window.clearInterval(timer);
            return next;
          }),
        200,
      );
      return () => window.clearInterval(timer);
    }, [attempt]);
    return (
      <div style={{ display: "grid", gap: 24, padding: 24 }}>
        <p>{progress < 100 ? `Uploading · ${progress}%` : "Upload complete"}</p>
        {[file, text].map((paste) => (
          <div key={paste.token} style={{ fontFamily: "var(--ui-font-mono)" }}>
            <PasteChip
              paste={{
                ...paste,
                attachment: {
                  ...paste.attachment!,
                  progress,
                  state: progress < 100 ? "uploading" : "ready",
                },
              }}
              onOpen={() => {}}
            />
          </div>
        ))}
        <Button
          onClick={() => {
            setProgress(0);
            setAttempt((value) => value + 1);
          }}
        >
          Replay upload
        </Button>
      </div>
    );
  },
};
