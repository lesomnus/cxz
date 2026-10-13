import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@lesomnus/cxz-ui";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { BackgroundJobsIndicator } from "./background-jobs-indicator";
import { useMemo, useRef } from "react";
import type { Connection } from "#src/shared/api/connection.ts";
import { useSessionBackground } from "#src/features/session/model/use-session-background.ts";
import { storyEvent } from "#src/storybook/fixtures.ts";

const meta = {
  title: "Conversation/BackgroundJobs",
  component: BackgroundJobsIndicator,
  tags: ["autodocs"],
  args: { count: 2 },
  parameters: {
    docs: {
      description: {
        component:
          "Active provider-reported background jobs. Each job lights another section of the border, up to a full perimeter. The exact count stays visible. The luminous outline drifts and breathes without a JavaScript animation loop or blur filter. Unknown telemetry is a dash, not a guessed zero.",
      },
    },
  },
} satisfies Meta<typeof BackgroundJobsIndicator>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Active: Story = {};
export const Idle: Story = { args: { count: 0 } };
export const Unknown: Story = { args: { count: undefined } };
export const FullBorder: Story = { args: { count: 12 } };
export const InToolbar: Story = {
  render: function Toolbar() {
    const [count, setCount] = useState(2);
    return (
      <>
        <div style={{ display: "flex", gap: 4 }}>
          <Button onClick={() => setCount((value) => value + 1)}>
            Start job
          </Button>
          <Button onClick={() => setCount((value) => Math.max(0, value - 1))}>
            Finish job
          </Button>
        </div>
        <ComponentPreview working backgroundJobs={count} />
      </>
    );
  },
  parameters: { layout: "fullscreen" },
};

export const LiveTelemetry: Story = {
  render: function Telemetry() {
    const connection = useMemo(
      () =>
        ({
          sessions: {
            background: async () => ({
              data: new TextEncoder().encode(
                JSON.stringify({ preview: { Snapshot: true, Tasks: {} } }),
              ),
              lastSeq: 0n,
            }),
          },
        }) as unknown as Connection,
      [],
    );
    const jobs = useSessionBackground(connection, "preview", "preview");
    const sequence = useRef(0);
    const active = useRef<string[]>([]);
    const emit = (data: Record<string, unknown>) => {
      const event = storyEvent(++sequence.current, "background", "", {
        type: "system",
        ...data,
      });
      event.runId = "preview";
      jobs.receive(event);
    };
    return (
      <>
        <div style={{ display: "flex", gap: 4 }}>
          <Button
            onClick={() => {
              const id = `task-${sequence.current + 1}`;
              active.current.push(id);
              emit({
                subtype: "task_started",
                task_id: id,
                is_backgrounded: true,
              });
            }}
          >
            Start job
          </Button>
          <Button
            onClick={() => {
              const id = active.current.pop();
              if (id)
                emit({
                  subtype: "task_notification",
                  task_id: id,
                  status: "completed",
                });
            }}
          >
            Finish job
          </Button>
        </div>
        <ComponentPreview backgroundJobs={jobs.count} />
      </>
    );
  },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        story:
          "Uses the connected UI's real background state hook with a fake initial RPC snapshot and native start/completion events. Jobs continue while the foreground turn is idle. No real session is changed.",
      },
    },
  },
};
