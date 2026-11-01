import { StrictMode, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, useTheme } from "@lesomnus/nox";
import { Markdown, MarkdownFileLinks } from "./markdown";

const snippet =
  "const greeting = '<script>alert(1)</script>';\nconsole.log(greeting);";
const example =
  "A code block keeps its original text while you choose a highlighting language.\n\n```javascript\n" +
  snippet +
  '\n```\n\n```\n{"ready": true, "count": 3}\n```\n\n```unknown-language\nraw <tag> & text\n```\n\n[Project documentation](https://github.com/lesomnus/cxz)';
const mermaidFlowchart =
  "flowchart LR\n  accTitle: Message lifecycle\n  Input[사용자 입력] --> Manager[Manager]\n  Manager --> Agent[Agent]\n  Agent --> Response[응답]";
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
          "Hover a code block to show its language chooser and copy icon just above its upper-right edge. The floating toolbar occupies no layout space and remains reachable across the gap. Keyboard focus also reveals controls. Language choices only affect highlighting; copy preserves the literal code without Markdown fences. Menus stay visible while open, and unknown languages fall back to plain text.",
      },
    },
  },
} satisfies Meta<typeof Markdown>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CodeBlocks: Story = {};
export const Mermaid: Story = {
  args: {
    text:
      "Mermaid fences are rendered locally as diagrams. Hover or focus for source and copy controls.\n\n```mermaid\n" +
      mermaidFlowchart +
      "\n```\n\n```mermaid\nsequenceDiagram\n  participant Browser\n  participant Manager\n  participant Agent\n  Browser->>Manager: Send message\n  Manager->>Agent: Run turn\n  Agent-->>Browser: Stream response\n```\n\nOrdinary code keeps its highlighting.\n\n```javascript\nconsole.log('ready');\n```",
  },
  parameters: {
    docs: {
      description: {
        story:
          "Mermaid is bundled and loaded only for Mermaid fences; no external rendering service is used. Diagrams use the app's grayscale theme tokens. Source and Copy preserve the original definition. Rendered SVG images isolate diagram styles and IDs from the conversation. Invalid or incomplete definitions keep their source visible, and large diagrams use the shared ScrollArea. Rendering is serialized, streaming is coalesced, and a bounded in-memory cache avoids repeating layout when virtual conversation rows remount.",
      },
    },
  },
};
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

function MermaidStreamingPreview() {
  const { theme, setPreference } = useTheme();
  const [source, setSource] = useState("flowchart LR\n  Input[");
  const [present, setPresent] = useState(true);
  const [extra, setExtra] = useState(false);
  return (
    <>
      <div style={{ display: "flex", gap: "var(--space)", flexWrap: "wrap" }}>
        <Button onClick={() => setSource(mermaidFlowchart)}>
          Complete diagram
        </Button>
        <Button
          onClick={() =>
            setSource(
              "flowchart LR\n  Latest[Latest definition] --> Done[Done]",
            )
          }
        >
          Replace diagram
        </Button>
        <Button onClick={() => setSource("flowchart LR\n  Broken[unfinished")}>
          Invalid diagram
        </Button>
        <Button onClick={() => setExtra((value) => !value)}>
          Append response
        </Button>
        <Button onClick={() => setPresent((value) => !value)}>
          {present ? "Remove diagram" : "Restore diagram"}
        </Button>
        <Button
          onClick={() => setPreference(theme === "dark" ? "light" : "dark")}
        >
          Toggle theme
        </Button>
      </div>
      <Markdown
        text={
          (present ? "```mermaid\n" + source + "\n```" : "No diagram.") +
          (extra ? "\n\nMore response text." : "")
        }
      />
    </>
  );
}
export const MermaidStreaming: Story = {
  render: () => <MermaidStreamingPreview />,
};

export const MermaidUntrusted: Story = {
  args: {
    text: 'The definition requests HTML, script links and page-level CSS. Site security remains in force.\n\n```mermaid\n---\nconfig:\n  securityLevel: loose\n  htmlLabels: true\n  themeCSS: \'body { display: none; }\'\n---\nflowchart LR\n  A["<img src=x onerror=alert(1)>"] --> B[Safe output]\n  click B "javascript:alert(1)"\n```',
  },
};

function FileLinksPreview() {
  const [path, openFile] = useState("");
  return (
    <MarkdownFileLinks.Provider
      value={{ open: openFile, size: async () => 12345n }}
    >
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
          "Container file chips inherit body typography and text color, with a transparent background, muted outline and vertical separator between file icon and label. The one-line tooltip uses Nox Popover and shows a fixed-width CompactNumber byte size before the container path on hover or keyboard focus. Size lookup is lazy and the session caches successful results briefly; production reads only the download size header before canceling. This story simulates file sizes. Session pages open absolute container paths, source references with line/column suffixes, and local file URIs in the workspace editor through the session's project API. External websites retain ordinary hyperlinks and open in a new tab; other URI schemes retain the sanitizer's restrictions.",
      },
    },
  },
};
