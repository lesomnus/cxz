import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@lesomnus/nox";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { BackgroundJobsIndicator } from "./background-jobs-indicator";
import { useMemo, useRef } from "react";
import { Connection } from "#src/shared/api/connection.ts";
import { createRouterTransport } from "@connectrpc/connect";
import { SessionService } from "#gen/cxz/session_svc_pb";
import { BackgroundReport } from "#src/features/activity/background-report.tsx";
import type { BackgroundTask } from "#src/features/activity/background-tasks.ts";
import { useSessionBackground } from "#src/features/session/model/use-session-background.ts";
import { storyEvent, sessionFixtureId } from "#src/storybook/fixtures.ts";

const meta = {
  title: "Conversation/BackgroundJobs",
  component: BackgroundJobsIndicator,
  tags: ["autodocs"],
  args: { count: 2 },
  parameters: {
    docs: {
      description: {
        component:
          "Active provider-reported background jobs grow a colored trail. Once that color nearly fills the perimeter, the next color starts growing over it. A soft highlight periodically travels within each segment while the outline drifts. The exact count stays visible. Both effects use the shared Nox BorderTrailButton without a JavaScript frame loop or surrounding glow. Unknown telemetry is a dash, not a guessed zero.",
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

function JobListPreview({ empty = false }: { empty?: boolean }) {
  const [fixture] = useState(() => {
    const tasks: BackgroundTask[] = empty
      ? []
      : [
          {
            task_id: "build",
            description:
              "Build the web application and verify generated assets",
            status: "working",
            active: true,
            summary: "Compiling the production bundle and checking types.",
            output_file: "/tmp/cxz/jobs/build.log",
          },
          {
            task_id: "watch",
            description: "Watch workspace files",
            status: "working",
            active: true,
            summary: "Waiting for changes in /workspace/ts/src.",
            output_file: "",
          },
          {
            task_id: "tests",
            description: "Run conversation tests",
            status: "completed",
            active: false,
            summary: "All conversation tests passed.",
            output_file: "/tmp/cxz/jobs/tests.log",
          },
          {
            task_id: "lint",
            description: "Check formatting",
            status: "failed",
            active: false,
            summary: "Formatting changes are needed in src/example.ts.",
            output_file: "/tmp/cxz/jobs/lint.log",
          },
        ];
    const transport = createRouterTransport((router) => {
      router.service(SessionService, {
        background: (request) => {
          if (request.ref?.key.case !== "id")
            throw new Error("Background requires a public session UUID");
          return {
            data: new TextEncoder().encode(
              JSON.stringify({
                preview: {
                  Snapshot: true,
                  Tasks: Object.fromEntries(
                    tasks.map((task) => [task.task_id, task]),
                  ),
                },
              }),
            ),
          };
        },
      });
    });
    return { c: new Connection(location.origin, transport), tasks };
  });
  return (
    <ComponentPreview
      backgroundJobs={fixture.tasks.filter((task) => task.active).length}
      backgroundReport={
        <BackgroundReport
          c={fixture.c}
          session={sessionFixtureId()}
          run="preview"
        />
      }
    />
  );
}

export const JobList: Story = {
  render: () => <JobListPreview />,
  argTypes: { count: { control: false } },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        story:
          "Click the background-job count in the composer toolbar to open the same floating card and BackgroundReport used in the session view. The in-memory RPC fixture includes running, completed and failed jobs, descriptions, summaries and output paths. Refresh reloads the fixture; Escape, Close or a conversation margin dismisses the card. No server is required.",
      },
    },
  },
};

export const EmptyJobList: Story = {
  ...JobList,
  render: () => <JobListPreview empty />,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        story:
          "Click the zero background-job count to inspect the real list card's empty state.",
      },
    },
  },
};

function Telemetry({ provider = "claude" }: { provider?: "claude" | "codex" }) {
  const [requests, setRequests] = useState(0);
  const connection = useMemo(
    () =>
      ({
        sessions: {
          background: async (request: {
            ref?: { key?: { case?: string } };
          }) => {
            if (request.ref?.key?.case !== "id")
              throw new Error("Background requires a public session UUID");
            setRequests((count) => count + 1);
            return {
              data: new TextEncoder().encode(
                JSON.stringify({ preview: { Snapshot: true, Tasks: {} } }),
              ),
              lastSeq: 0n,
            };
          },
        },
      }) as unknown as Connection,
    [],
  );
  const jobs = useSessionBackground(connection, sessionFixtureId(), "preview");
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
  const emitCodex = (unavailable = false) =>
    emit({
      type: "background",
      provider: "codex",
      subtype: unavailable ? "background_status" : "background_tasks_changed",
      status: unavailable ? "error" : undefined,
      tasks: active.current.map((task_id) => ({ task_id })),
    });
  return (
    <>
      <div
        data-background-requests={requests}
        style={{ display: "flex", gap: 4 }}
      >
        <Button
          onClick={() => {
            const id = `task-${sequence.current + 1}`;
            active.current.push(id);
            if (provider === "codex") {
              emitCodex();
              return;
            }
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
            if (provider === "codex") {
              emitCodex();
              return;
            }
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
        {provider === "codex" && (
          <>
            <Button onClick={() => emitCodex(true)}>
              Simulate unavailable
            </Button>
            <Button onClick={() => emitCodex()}>Refresh terminals</Button>
          </>
        )}
      </div>
      <ComponentPreview backgroundJobs={jobs.count} />
    </>
  );
}

export const LiveTelemetry: Story = {
  render: () => <Telemetry />,
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

export const CodexTelemetry: Story = {
  render: () => <Telemetry provider="codex" />,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        story:
          "Codex's supervisor publishes complete background-terminal snapshots through the real UI state hook. An empty list is zero; an unavailable read is unknown. Recovering refreshes replace the count, independently of the foreground turn.",
      },
    },
  },
};
