import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { SessionNavigationButton } from "./session-navigation-button";

function Samples() {
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => <>{children}</>,
        view: () => (
          <div style={{ display: "grid", gap: 16, padding: 24 }}>
            {[90, 160, 240].map((width) => (
              <div
                key={width}
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(2, max-content)",
                  gap: 8,
                }}
              >
                {["codex", "claude"].map((agent) => (
                  <div key={agent} style={{ width }}>
                    <SessionNavigationButton
                      session={create(SessionSchema, {
                        agent,
                        name: "Review shared project work",
                        alias: agent === "codex" ? "oak" : "elm",
                        runtimeId: agent === "codex" ? "oak" : "elm",
                      })}
                      annotation={width === 240 ? "Previous run" : undefined}
                    />
                  </div>
                ))}
              </div>
            ))}
          </div>
        ),
      },
      createMemoryHistory({ initialEntries: ["/c/_/session"] }),
    ),
  );
  return <RouterProvider router={router} />;
}

const meta = {
  title: "Components/SessionNavigationButton",
  component: Samples,
  parameters: {
    docs: {
      description: {
        component:
          "One shared session resource card for detail fields and tables. Logo spacing, name/alias typography and navigation are owned by the component; its container supplies the available width. Narrow, regular and wide examples include both agents and ownership annotations.",
      },
    },
  },
} satisfies Meta<typeof Samples>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Widths: Story = {};
