import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Connection } from "#src/shared/api/connection.ts";
import type { ProjectTerminalRequest } from "#gen/cxz/project_svc_pb";
import { TerminalWorkbench } from "./terminal-workbench";
import { TerminalSession } from "./terminal-session";
import { Button } from "@lesomnus/nox";

// Every stream has its own line/buffer. This is deliberately a local fake shell.
const connection = {
  inProcess: true,
  projects: {
    async *terminal(requests: AsyncIterable<ProjectTerminalRequest>) {
      const encode = (text: string) => new TextEncoder().encode(text);
      let opened = false,
        line = "";
      for await (const request of requests) {
        if (!opened) {
          opened = true;
          yield {
            ready: true,
            output: encode(
              "Storybook shell · no server connection\r\n/workspace$ ",
            ),
          };
        }
        if (!request.input?.length) continue;
        for (const char of new TextDecoder().decode(request.input)) {
          if (char === "\r" || char === "\n") {
            yield {
              output: encode(
                `\r\n${line.startsWith("echo ") ? line.slice(5) : line ? `Preview: ${line}` : ""}\r\n/workspace$ `,
              ),
            };
            line = "";
          } else if (char === "\x7f") {
            line = line.slice(0, -1);
            yield { output: encode("\b \b") };
          } else {
            line += char;
            yield { output: encode(char) };
          }
        }
      }
    },
  },
} as unknown as Connection;
const meta = {
  title: "Workspace/Terminal",
  component: TerminalWorkbench,
  args: { visible: true, hide: () => {}, renderSession: () => null },
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Add/close independent terminal tabs. The add button follows the last tab. Drag tabs within a header to reorder, to a pane's center to merge, or to its edges to split. Resize pane separators or the upper edge of the terminal area. Folding and docking preserve each shell's screen and connection. This story uses real xterm.js with a local simulated shell.",
      },
    },
  },
  render: function Example() {
    const [visible, setVisible] = useState(true);
    return (
      <div style={{ height: "85vh", display: "flex", flexDirection: "column" }}>
        <div style={{ flex: 1, padding: 16 }}>
          <Button onClick={() => setVisible((value) => !value)}>
            Toggle terminal
          </Button>
          <p>
            Try multiple tabs, nested splits, and drag a tab to another pane.
          </p>
        </div>
        <TerminalWorkbench
          visible={visible}
          hide={() => setVisible(false)}
          renderSession={(props) => (
            <TerminalSession {...props} c={connection} runtimeId="preview" />
          )}
        />
      </div>
    );
  },
} satisfies Meta<typeof TerminalWorkbench>;
export default meta;
export const Docking: StoryObj<typeof meta> = {};
