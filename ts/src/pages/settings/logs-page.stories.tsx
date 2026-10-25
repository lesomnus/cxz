import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
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
}: {
  persistent?: boolean;
  empty?: boolean;
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
    return dispose;
  }, [logs, empty, persistent]);
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
  title: "Settings/Logs",
  component: Preview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Memory: Story = {};
export const Persistent: Story = { args: { persistent: true } };
export const Empty: Story = { args: { empty: true } };
