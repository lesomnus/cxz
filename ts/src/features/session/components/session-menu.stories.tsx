import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { userEvent, within } from "storybook/test";
import {
  FloatingCardHost,
  FloatingCardProvider,
} from "#src/features/session/cards/floating-card.tsx";
import { storySession } from "#src/storybook/fixtures.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { SessionMenu } from "./session-menu";
import styles from "./session-menu.stories.module.css";

function menuSession(state: string, agent: string) {
  const session = storySession();
  session.agent = agent;
  session.status!.state = state;
  session.status!.runId = "storybook-menu-run";
  return session;
}

function MenuPreview({
  state,
  busy,
  agent,
  alwaysOpen,
}: {
  state: "idle" | "working" | "stopped";
  busy: boolean;
  agent: "codex" | "claude";
  alwaysOpen: boolean;
}) {
  const [session, setSession] = useState(() => menuSession(state, agent));
  const [terminal, setTerminal] = useState(false);
  const [action, setAction] = useState("");
  useEffect(() => {
    setSession(menuSession(state, agent));
    setTerminal(false);
    setAction("");
  }, [state, agent]);
  const tools = [
    "Insert secret",
    "Shared memory",
    "Auxiliary AI",
    "Conversation usage",
    "Background tasks",
    ...(agent === "claude" ? ["Log in"] : []),
  ];
  return (
    <FloatingCardProvider>
      <div className={`conversation ${styles.preview}`}>
        <div className="transcript-area" />
        <FloatingCardHost />
        <div className={`composer-wrapper ${styles.wrapper}`}>
          <div className="composer-toolbar">
            <SessionMenu
              session={session}
              info={{ model: session.model, effort: "medium" }}
              busy={busy}
              open={alwaysOpen ? true : undefined}
              terminal={{
                visible: terminal,
                available: true,
                toggle: () => setTerminal((value) => !value),
              }}
              tools={tools.map((label) => ({
                label,
                disabled:
                  label === "Insert secret"
                    ? busy
                    : label === "Log in" &&
                      (busy || session.status?.state === "working"),
                run: () => setAction(label),
              }))}
              upload={(directory) =>
                setAction(directory ? "Upload folder" : "Upload files")
              }
              fork={() => setAction("Fork")}
              manage={async (operation) => {
                setSession((session) => ({
                  ...session,
                  status: {
                    ...session.status!,
                    state: operation === "stop" ? "stopped" : "idle",
                  },
                }));
                setAction(operation);
                return true;
              }}
              edit={async (field, value) => {
                const updated = { ...session, [field]: value };
                setSession(updated);
                return updated;
              }}
            />
          </div>
        </div>
        <output className={styles.action} aria-label="Last action">
          {action}
        </output>
      </div>
    </FloatingCardProvider>
  );
}

const meta = {
  title: "Conversation/Session menu",
  component: MenuPreview,
  tags: ["autodocs"],
  args: { state: "idle", busy: false, agent: "codex", alwaysOpen: false },
  argTypes: {
    state: { control: "select", options: ["idle", "working", "stopped"] },
    agent: { control: "select", options: ["codex", "claude"] },
  },
  parameters: {
    docs: {
      description: {
        component:
          "The actual composer SessionMenu, without a transcript or editor. The menu opens automatically for design inspection. Session actions, tools, uploads and the fixed editor-option footer use the production components. Actions are simulated locally; editor options use the Storybook origin's saved settings. Change the theme toolbar to compare dark and light surfaces.",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const button = within(canvasElement).getByRole("button", {
      name: t("Session menu"),
    });
    if (button.getAttribute("aria-expanded") !== "true")
      await userEvent.click(button);
  },
} satisfies Meta<typeof MenuPreview>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const AlwaysOpen: Story = {
  args: { alwaysOpen: true },
  parameters: {
    docs: {
      description: {
        story:
          "Keeps the real menu mounted and open for DevTools inspection. Outside clicks, focus changes, Escape, trigger clicks and action selection cannot dismiss it. Actions and editor options remain interactive.",
      },
    },
  },
};
export const Working: Story = { args: { state: "working" } };
export const Stopped: Story = { args: { state: "stopped" } };
export const Busy: Story = { args: { busy: true } };
export const Claude: Story = { args: { agent: "claude" } };
