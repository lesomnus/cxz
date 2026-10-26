import { StrictMode, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@lesomnus/nox";
import { Markdown, MarkdownFileLinks } from "./markdown";

const snippet =
  "const greeting = '<script>alert(1)</script>';\nconsole.log(greeting);";
const example =
  "A code block keeps its original text while you choose a highlighting language.\n\n```javascript\n" +
  snippet +
  '\n```\n\n```\n{"ready": true, "count": 3}\n```\n\n```unknown-language\nraw <tag> & text\n```\n\n[Project documentation](https://github.com/lesomnus/cxz)';
const meta = {
  title: "Content/Markdown",
  component: Markdown,
  tags: ["autodocs"],
  args: { text: example },
  decorators: [
    (Story) => (
      <StrictMode>
        <div style={{ width: "min(600px, 90vw)" }}>
          <Story />
        </div>
      </StrictMode>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Hover a code block to show its language chooser and copy icon at the upper right. Keyboard focus also reveals controls. Language choices only affect highlighting; copy preserves the literal code without Markdown fences. Menus stay visible while open, and unknown languages fall back to plain text.",
      },
    },
  },
} satisfies Meta<typeof Markdown>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CodeBlocks: Story = {};
export const Tables: Story = {
  args: {
    text: "| 값 | 수 | | 상태 |\n| --- | --- | --- | --- |\n| 가 | 1 | | 확인됨 |\n| 한글자 | 2 | | 대기 |\n\n| Name | Count | Description |\n| --- | ---: | --- |\n| API | 3 | Longer content can still determine a column's width. |",
  },
};
function StreamingPreview() {
  const [extra, setExtra] = useState(false);
  return (
    <>
      <Button onClick={() => setExtra(true)}>Append response</Button>
      <Markdown
        text={
          example +
          (extra
            ? "\n\nMore response text.\n\n```python\nprint('next block')\n```"
            : "")
        }
      />
    </>
  );
}
export const Streaming: Story = { render: () => <StreamingPreview /> };

function FileLinksPreview() {
  const [path, openFile] = useState("");
  return (
    <MarkdownFileLinks.Provider value={openFile}>
      <Markdown text="[Report](/tmp/report.md) · [Source line](/workspace/file.ts:42) · [File URI](file:///workspace/file.ts) · [File with spaces](file:///tmp/a%20report.md) · [External documentation](https://example.com/) · [Network URL](//example.com/docs) · [Unsafe link](javascript:alert%281%29)" />
      <output aria-label="Opened file">{path || "Select a file link."}</output>
    </MarkdownFileLinks.Provider>
  );
}
export const FileLinks: Story = {
  render: () => <FileLinksPreview />,
  parameters: {
    docs: {
      description: {
        story:
          "Container file links appear as compact chips with a small file icon, distinct from ordinary underlined web links. Session pages open absolute container paths, source references with line/column suffixes, and local file URIs in the workspace editor through the session's project API. The browser's resolved link preview is not used as a web navigation URL. This preview displays the path passed to that callback. External websites still open in a new tab; other URI schemes retain the sanitizer's restrictions.",
      },
    },
  },
};
