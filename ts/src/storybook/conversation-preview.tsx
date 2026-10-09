import { useRef, useState, type ReactNode } from "react";
import type { SessionEvent } from "../../gen/cxz/session_pb";
import { FloatingCardHost, FloatingCardProvider } from "../floating-card";
import { Transcript } from "../transcript";
import { EventView } from "../event-view";
import { transcriptEvents } from "../tool-activity";
import { ConversationComposer } from "../conversation-composer";
import { createPaste, type ComposerPaste } from "../composer-pastes";
import { sessionCommands } from "../composer-commands";
import { ModelSettings, type ModelCatalog } from "../model-settings";
import { UsageInfo } from "../usage-info";
import { storySession } from "./fixtures";
import type { SessionInfo } from "../session-info";
import type { TurnProgress } from "../turn-progress";
import "./preview.css";

export const samplePaste = createPaste(
  "export function greet(name: string) {\n  return `Hello, ${name}`;\n}\nconsole.log(greet('Storybook'));",
  "a1b2c3d4",
);
export function previewPastes() {
  return new Map([[samplePaste.token, samplePaste]]);
}
export const idleTurn: TurnProgress = { runId: "storybook", active: false };
export const noop = () => {};

export function PreviewMetadata({
  busy = false,
  agent = "codex",
}: {
  busy?: boolean;
  agent?: string;
}) {
  const [model, setModel] = useState(
    agent === "claude" ? "claude-sonnet" : "gpt-6.1",
  );
  const [effort, setEffort] = useState("medium");
  const catalog: ModelCatalog = {
    model,
    effort,
    models: [
      {
        id: agent === "claude" ? "claude-sonnet" : "gpt-6.1",
        efforts: ["low", "medium", "high"],
      },
      {
        id: agent === "claude" ? "claude-haiku" : "gpt-6.1-mini",
        efforts: ["low", "medium", "high"],
      },
    ],
  };
  const info: SessionInfo = {
    model,
    effort,
    remaining: 72,
    reset: Date.now() + 3600_000,
    quotaLabel: "Session",
    contextUsed: 48_000,
    contextWindow: 200_000,
  };
  const session = storySession();
  session.agent = agent;
  session.status!.state = busy ? "working" : "idle";
  return (
    <>
      <ModelSettings
        session={session}
        info={info}
        catalog={catalog}
        busy={busy}
        change={(kind, value) =>
          kind === "model" ? setModel(value) : setEffort(value)
        }
      />
      <UsageInfo info={info} />
    </>
  );
}

export function PreviewFrame({ children }: { children: ReactNode }) {
  return (
    <FloatingCardProvider>
      <div className="storybook-conversation conversation">{children}</div>
    </FloatingCardProvider>
  );
}

// Keep real virtualization, hover handles, prompt peeking and anchored details.
export function PreviewTranscript({
  events,
  agent = "codex",
  pane,
  follow,
  navigate = noop,
  reading = noop,
}: {
  events: SessionEvent[];
  agent?: string;
  pane: React.RefObject<HTMLDivElement | null>;
  follow: React.RefObject<boolean>;
  navigate?: () => void;
  reading?: (reading: boolean) => void;
}) {
  const projected = transcriptEvents(events);
  const [jump, setJump] = useState<string>();
  return (
    <Transcript
      pane={pane}
      events={projected.events}
      follow={follow}
      render={(e) => (
        <EventView
          e={e}
          agent={agent}
          activity={projected.activities.get(e.seq)}
          completion={projected.completions.get(e.seq.toString())}
        />
      )}
      notice={null}
      onNavigate={navigate}
      onScroll={reading}
      onTension={noop}
      onMotion={noop}
      onReadingMove={noop}
      precedingPrompt={undefined}
      jumpTarget={jump}
      onPromptJump={setJump}
      onJumped={() => setJump(undefined)}
      older={noop}
      newer={noop}
    />
  );
}

export function ComponentPreview({
  events = [],
  agent = "codex",
  value = "",
  onChange,
  sending = false,
  working = false,
  children,
}: {
  events?: SessionEvent[];
  agent?: string;
  value?: string;
  onChange?: (value: string) => void;
  sending?: boolean;
  working?: boolean;
  children?: ReactNode;
}) {
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const pastes = useRef<Map<string, ComposerPaste>>(previewPastes());
  const startedAt = useRef(Date.now() - 14_000);
  const [stopped, setStopped] = useState(false);
  const active = working && !stopped;
  return (
    <PreviewFrame>
      <PreviewTranscript
        events={events}
        agent={agent}
        pane={pane}
        follow={follow}
      />
      <FloatingCardHost>{children}</FloatingCardHost>
      <ConversationComposer
        draft={value}
        onChange={onChange ?? noop}
        pastes={pastes.current}
        commands={sessionCommands(agent)}
        onSubmit={(e) => e.preventDefault()}
        canSend={false}
        sending={sending}
        busy={sending}
        turn={
          active
            ? {
                runId: "storybook",
                active: true,
                startedAt: startedAt.current,
                key: "preview-turn",
              }
            : idleTurn
        }
        working={active}
        interrupt={() => setStopped(true)}
      >
        <PreviewMetadata agent={agent} busy={sending || active} />
      </ConversationComposer>
    </PreviewFrame>
  );
}
