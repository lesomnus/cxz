import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { Button } from "@lesomnus/nox";
import { PasteChip } from "./paste-chip";
import { isImagePaste, type ComposerPaste } from "./composer-pastes";
import { ImagePastePreview } from "./image-paste-preview";
import {
  previewImage,
  readPreviewImage,
} from "#src/storybook/image-preview-fixture.ts";
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
const image: ComposerPaste = {
  token: "[File 87654321 · image.svg]",
  body: "",
  lines: 0,
  bytes: 2500000,
  attachment: {
    name: "image.svg",
    file: previewImage(),
    mediaType: "image/svg+xml",
    state: "uploading",
    progress: 35,
  },
};
const meta = {
  title: "Composer/Upload chips",
  component: PasteChip,
  args: { paste: file, onOpen: () => {} },
  render: function Chip(args) {
    const [paste, setPaste] = useState(args.paste);
    const [preview, setPreview] = useState(false);
    useEffect(() => setPaste(args.paste), [args.paste]);
    return (
      <>
        <PasteChip
          {...args}
          paste={paste}
          canSendFile
          onSendModeChange={(mode) => setPaste({ ...paste, sendAs: mode })}
          onOpen={() => {
            args.onOpen();
            if (isImagePaste(paste)) setPreview(true);
          }}
        />
        {preview && (
          <ImagePastePreview
            paste={paste}
            download={readPreviewImage}
            close={() => setPreview(false)}
          />
        )}
      </>
    );
  },
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
export const UploadingImage: Story = { args: { paste: image } };
export const ReadyImage: Story = {
  args: {
    paste: { ...image, attachment: { ...image.attachment!, state: "ready" } },
  },
};
export const RestoredImage: Story = {
  args: {
    paste: {
      ...image,
      attachment: {
        ...image.attachment!,
        file: undefined,
        path: "/cxz/assets/storybook/upload/image.svg",
        state: "ready",
      },
    },
  },
  parameters: {
    docs: {
      description: {
        story:
          "The File is absent, like a restored draft. Opening reads the container path through the same download callback used by the live composer; this story supplies an in-browser fixture. 100% fits the image, and the footer slider zooms around its center. Drag or use arrow keys in the image viewport to pan when enlarged.",
      },
    },
  },
};
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
          "Paste up to 32 characters as ordinary text, 33–64 as a Text chip, and longer text as a File chip by default. Text chips upload only when switched to File. You can also drop files and folders into the real conversation editor. Hover shows a short excerpt. Clicking a paste label opens its complete source in a headerless Card above the composer, with Expand on the left and Close on the right. Delivery controls only change Text/File mode. Backspace/Delete removes a selected chip, and Ctrl+P also opens its preview. Native paste/drop runs normally; only upload acknowledgement is simulated, so no files leave the browser.",
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
    const [sendAs, setSendAs] = useState<"text" | "file">("file");
    const [preview, setPreview] = useState<ComposerPaste>();
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
        {[file, text, image].map((paste) => (
          <div key={paste.token} style={{ fontFamily: "var(--ui-font-mono)" }}>
            <PasteChip
              paste={{
                ...paste,
                sendAs: paste.body ? sendAs : undefined,
                attachment: {
                  ...paste.attachment!,
                  progress,
                  state: progress < 100 ? "uploading" : "ready",
                },
              }}
              onOpen={() => {
                if (isImagePaste(paste)) setPreview(paste);
              }}
              canSendFile
              onSendModeChange={setSendAs}
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
        {preview && (
          <ImagePastePreview
            paste={preview}
            download={readPreviewImage}
            close={() => setPreview(undefined)}
          />
        )}
      </div>
    );
  },
};
