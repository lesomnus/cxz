import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Button } from "@lesomnus/nox";
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
          session: "01949770-0000-8000-8008-000000000001",
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
      <LogsPage logs={logs} />
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
          "Browser diagnostics in the same centered, bounded layout as resource pages. The shared FindBar searches retained logs; level, category and identity filters remain available. Dense, single-line table rows open a shared modal with the complete record, including the method and category, without expanding the table. Live updates, OPFS persistence, filtered JSONL downloads and pagination use the actual log store.",
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
