import { useMemo, useRef, useState, useSyncExternalStore } from "react";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { Button } from "@lesomnus/nox";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ClientErrors } from "./client-errors";
import type { ErrorOrigin } from "./error-origin";
import { ErrorTable } from "./error-table";
import { ErrorPanel } from "./error-panel";
import { errorRowExamples } from "./error-row-examples";

function RowCatalog({
  kind = "all",
  dialog = false,
  live = false,
}: {
  kind?: ErrorOrigin["kind"] | "all";
  dialog?: boolean;
  live?: boolean;
}) {
  const [store] = useState(() => {
    const store = new ClientErrors();
    if (live)
      store.report("initial", "Existing error", {
        origin: {
          kind: "rpc",
          service: "cxz.SessionService",
          method: "Transcript",
        },
        code: "Unavailable",
        subject: { kind: "session", alias: "existing" },
      });
    else
      for (const item of errorRowExamples)
        if (kind === "all" || item.context.origin?.kind === kind)
          store.report(item.id, item.message, item.context);
    return store;
  });
  const entries = useSyncExternalStore(
    store.subscribe,
    store.snapshot,
    store.snapshot,
  );
  const [tab, setTab] = useState("active");
  const sequence = useRef(0);
  const report = (n: number) =>
    store.report(`live-${n}`, `New error ${n}`, {
      origin: {
        kind: "rpc",
        service: "cxz.SessionService",
        method: "Transcript",
      },
      code: "Unavailable",
      subject: { kind: "session", alias: `session-${n}` },
    });
  return (
    <main
      style={{ padding: 24, maxWidth: dialog ? 468 : 1000, margin: "0 auto" }}
    >
      {live && (
        <div style={{ display: "flex", gap: 8, marginBottom: 12 }}>
          <Button
            onClick={() => {
              report(++sequence.current);
            }}
          >
            Add error
          </Button>
          <Button
            disabled={!sequence.current}
            onClick={() => {
              report(sequence.current);
            }}
          >
            Repeat latest error
          </Button>
        </div>
      )}
      {dialog ? (
        <ErrorPanel store={store} tab={tab} onTabChange={setTab} />
      ) : (
        <ErrorTable entries={entries} store={store} tab="all" />
      )}
    </main>
  );
}
function Preview({
  kind = "all",
  dialog = false,
  live = false,
}: {
  kind?: ErrorOrigin["kind"] | "all";
  dialog?: boolean;
  live?: boolean;
}) {
  const router = useMemo(
    () =>
      createWorkspaceRouter(
        {
          shell: ({ children }) => <>{children}</>,
          view: () => {
            const route = useWorkspaceRoute();
            return (
              <RowCatalog
                kind={kind}
                live={live}
                dialog={dialog && route.resource !== "errors"}
              />
            );
          },
        },
        createMemoryHistory({ initialEntries: ["/c/_/session"] }),
      ),
    [kind, dialog, live],
  );
  return <RouterProvider router={router} />;
}
const meta = {
  title: "Workspace/Errors/Rows",
  component: Preview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const All: Story = {};
export const Rpc: Story = { name: "RPC", args: { kind: "rpc" } };
export const Http: Story = { name: "HTTP", args: { kind: "http" } };
export const Connection: Story = { args: { kind: "connection" } };
export const Runtime: Story = { args: { kind: "runtime" } };
export const Client: Story = { args: { kind: "client" } };
export const Dialog: Story = { args: { dialog: true } };

export const RpcDetails: Story = {
  name: "RPC details",
  args: { kind: "rpc" },
  play: async ({ canvasElement }) => {
    const row = [...canvasElement.querySelectorAll("tr")].find((row) =>
      row.textContent?.includes("cxz.SessionService/Transcript"),
    );
    row?.scrollIntoView({ block: "start" });
    row
      ?.querySelector<HTMLButtonElement>('button[aria-haspopup="dialog"]')
      ?.click();
  },
};

export const RpcDialog: Story = {
  name: "RPC dialog",
  args: { kind: "rpc", dialog: true },
};
export const HttpDialog: Story = {
  name: "HTTP dialog",
  args: { kind: "http", dialog: true },
};
export const ConnectionDialog: Story = {
  args: { kind: "connection", dialog: true },
};
export const RuntimeDialog: Story = { args: { kind: "runtime", dialog: true } };
export const ClientDialog: Story = { args: { kind: "client", dialog: true } };

export const IncomingErrors: Story = {
  args: { dialog: true, live: true },
  parameters: {
    docs: {
      description: {
        story:
          "Add error creates a new episode that slides down from above. Repeating the same error updates its count without replaying the entrance. Existing rows and tab switches stay still.",
      },
    },
  },
};

export const IncomingPage: Story = { args: { live: true } };
