import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Button } from "@lesomnus/nox";
import { create } from "@bufbuild/protobuf";
import { parse as parseId } from "@lesomnus/payday/pdid";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { resourceFixture } from "#src/storybook/resource-fixture.ts";
import { LogsPage } from "./logs-page";
import {
  ClientLogs,
  clientLogs,
  installClientLogs,
} from "#src/shared/logs/client-logs.ts";
import { MemoryLogStorage } from "#src/shared/logs/log-storage.ts";

function Preview({
  persistent = false,
  empty = false,
  populated = false,
}: {
  persistent?: boolean;
  empty?: boolean;
  populated?: boolean;
}) {
  const [c] = useState(() => resourceFixture());
  const [projects] = useState(() => [
    create(ProjectSchema, {
      id: parseId("01949770-0000-8000-8008-000000000002").bytes,
      name: "Workspace",
      alias: "workspace",
    }),
    create(ProjectSchema, {
      id: parseId("01949770-0000-8000-8008-000000000004").bytes,
      name: "Archive",
      alias: "archive",
    }),
  ]);
  const [sessions] = useState(() => [
    create(SessionSchema, {
      id: parseId("01949770-0000-8000-8008-000000000001").bytes,
      name: "Review",
      alias: "review",
      runtimeId: "review",
      project: { id: projects[0].id },
    }),
    create(SessionSchema, {
      id: parseId("01949770-0000-8000-8008-000000000003").bytes,
      name: "Build",
      alias: "build",
      runtimeId: "build",
      project: { id: projects[0].id },
    }),
  ]);
  useEffect(() => () => c.dispose(), [c]);
  const [logs] = useState(() =>
    persistent
      ? clientLogs()
      : new ClientLogs(new MemoryLogStorage(), "storybook"),
  );
  const [writing, setWriting] = useState(false);
  async function burst() {
    setWriting(true);
    try {
      for (let batch = 0; batch < 12; batch++) {
        for (let i = 0; i < 100; i++)
          logs.write({
            level: "info",
            category: "client",
            message: `Rotation record ${batch * 100 + i}: ` + "x".repeat(2000),
          });
        await logs.flush();
      }
    } finally {
      setWriting(false);
    }
  }
  useEffect(() => {
    const dispose = persistent ? installClientLogs() : () => {};
    if (!empty)
      logs.write({
        level: "info",
        category: "rpc",
        message: "RPC connection recovered",
        connection: "https://cxz.example",
        session: "01949770-0000-8000-8008-000000000001",
        method: "cxz.SessionService/Events",
        details: { disconnectedMs: 1200 },
      });
    if (populated) {
      for (let i = 0; i < 24; i++) {
        logs.write({
          level: i % 7 === 0 ? "error" : i % 3 === 0 ? "warn" : "info",
          category: i % 2 === 0 ? "rpc" : "client",
          message:
            i % 7 === 0
              ? `Request ${i + 1} failed while reading conversation history`
              : `Diagnostic record ${i + 1}: session state synchronized`,
          connection: "https://cxz.example",
          project: "01949770-0000-8000-8008-000000000002",
          session:
            i % 2 === 0
              ? "01949770-0000-8000-8008-000000000001"
              : "01949770-0000-8000-8008-000000000003",
          method: i % 2 === 0 ? "cxz.SessionService/Transcript" : undefined,
          details: {
            attempt: i + 1,
            elapsedMs: 120 + i * 17,
            message: "The complete diagnostic record opens in a modal.",
          },
        });
      }
    }
    return dispose;
  }, [logs, empty, persistent, populated]);
  return (
    <div
      style={{
        width: "100vw",
        height: "100vh",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div style={{ display: "flex", gap: 4, padding: 8 }}>
        <Button
          onClick={() =>
            logs.write({
              level: "warn",
              category: "rpc",
              message: "RPC stream disconnected",
              connection: "https://cxz.example",
              session: "01949770-0000-8000-8008-000000000001",
              method: "cxz.SessionService/Events",
              details: {
                closeCode: 1006,
                closeReason: "Connection interrupted",
                wasClean: false,
              },
            })
          }
        >
          Record disconnect
        </Button>
        <Button
          onClick={() =>
            window.dispatchEvent(
              new ErrorEvent("error", {
                message: "Client rendering failed",
                error: new Error("Client rendering failed"),
              }),
            )
          }
        >
          Record client error
        </Button>
        <Button disabled={writing} onClick={burst}>
          Generate rotation burst
        </Button>
      </div>
      <LogsPage logs={logs} c={c} projects={projects} sessions={sessions} />
    </div>
  );
}
const meta = {
  title: "Pages/Logs",
  component: Preview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Browser diagnostics in the centered resource layout. Project/session selectors share the resource selection state. The shared FindBar contains checkbox menus for multiple log levels and categories. Dense, single-line rows use level icons and omit timezone offsets; complete records open in a modal. History loads automatically through stable ID cursors, and only viewport rows plus overscan are mounted. Live updates follow the head while it is visible; history remains stable while reading older records. Use Generate rotation burst to exercise continuous scrolling. OPFS persistence and filtered downloads use the actual log store.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Memory: Story = {};
export const Persistent: Story = { args: { persistent: true } };
export const Empty: Story = { args: { empty: true } };
export const Populated: Story = { args: { populated: true } };
export const Details: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const record = await canvas.findByRole(
      "button",
      { name: "RPC connection recovered" },
      { timeout: 10000 },
    );
    await userEvent.click(record);
  },
};
