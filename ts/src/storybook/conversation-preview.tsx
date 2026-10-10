import { useRef, useState, type ReactNode } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import {
  FloatingCardHost,
  FloatingCardProvider,
} from "#src/features/session/cards/floating-card.tsx";
import { Transcript } from "#src/features/session/conversation/transcript.tsx";
import { EventView } from "#src/features/session/conversation/event-view.tsx";
import { transcriptEvents } from "#src/features/session/model/tool-activity.ts";
import { ConversationComposer } from "#src/features/session/composer/conversation-composer.tsx";
import {
  createPaste,
  type ComposerPaste,
} from "#src/features/session/composer/composer-pastes.ts";
import { sessionCommands } from "#src/features/session/composer/composer-commands.ts";
import {
  ModelSettings,
  type ModelCatalog,
} from "#src/features/session/composer/model-settings.tsx";
import { UsageInfo } from "#src/features/session/composer/usage-info.tsx";
import { storySession } from "./fixtures";
import { SessionMenu } from "#src/features/session/components/session-menu.tsx";
import { create } from "@bufbuild/protobuf";
import { SessionPurgeReplySchema } from "#gen/cxz/session_svc_pb";
import type { SessionInfo } from "#src/features/session/model/session-info.ts";
import type { TurnProgress } from "#src/features/session/model/turn-progress.ts";
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
  sessionStopped = false,
  uploadMode,
  children,
}: {
  events?: SessionEvent[];
  agent?: string;
  value?: string;
  onChange?: (value: string) => void;
  sending?: boolean;
  working?: boolean;
  sessionStopped?: boolean;
  uploadMode?: "ready" | "slow" | "retry";
  children?: ReactNode;
}) {
  const pane = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const pastes = useRef<Map<string, ComposerPaste>>(previewPastes());
  const startedAt = useRef(Date.now() - 14_000);
  const [stopped, setStopped] = useState(sessionStopped);
  const [purged, setPurged] = useState(false);
  const [generation, setGeneration] = useState(1);
  const active = working && !stopped;
  const [identity, setIdentity] = useState({
    name: storySession().name,
    alias: storySession().alias,
  });
  const identityRef = useRef(identity);
  const attempts = useRef(new Set<string>());
  const previewUpload = async (file: File, signal: AbortSignal) => {
    await new Promise<void>((resolve, reject) => {
      if (signal.aborted) {
        reject(signal.reason);
        return;
      }
      const abort = () => {
        clearTimeout(timer);
        reject(signal.reason);
      };
      const timer = setTimeout(
        () => {
          signal.removeEventListener("abort", abort);
          resolve();
        },
        uploadMode === "slow" ? 3000 : 500,
      );
      signal.addEventListener("abort", abort, { once: true });
    });
    const key = file.name + ":" + file.size;
    if (uploadMode === "retry" && !attempts.current.has(key)) {
      attempts.current.add(key);
      throw new Error("Simulated upload failure. Retry from the chip.");
    }
    return `/cxz/assets/storybook/upload/${file.name}`;
  };
  const session = storySession();
  Object.assign(session, identity);
  session.agent = agent;
  session.status!.state = stopped ? "stopped" : active ? "running" : "idle";
  session.status!.runId = `storybook-run-${generation}`;
  return (
    <PreviewFrame>
      <PreviewTranscript
        events={events}
        agent={agent}
        pane={pane}
        follow={follow}
        navigate={() => {
          follow.current = false;
        }}
      />
      <FloatingCardHost>{children}</FloatingCardHost>
      <ConversationComposer
        draft={value}
        onChange={onChange ?? noop}
        pastes={pastes.current}
        commands={sessionCommands(agent)}
        onSubmit={(e) => e.preventDefault()}
        canSend={!!uploadMode && !!value.trim() && !sending}
        upload={uploadMode ? previewUpload : undefined}
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
        menu={(pick) => (
          <SessionMenu
            session={purged ? undefined : session}
            info={{
              model: session.model,
              effort: "medium",
              remaining: 72,
              contextUsed: 48000,
              contextWindow: 200000,
            }}
            busy={sending}
            upload={uploadMode ? pick : undefined}
            edit={async (field, value) => {
              identityRef.current = { ...identityRef.current, [field]: value };
              setIdentity(identityRef.current);
              return { ...session, ...identityRef.current };
            }}
            manage={async (operation) => {
              setStopped(operation === "stop");
              if (operation !== "stop") setGeneration((value) => value + 1);
              return true;
            }}
            previewPurge={async () =>
              create(SessionPurgeReplySchema, {
                dryRun: true,
                targets: [
                  {
                    kind: "journal",
                    path: "/storybook/conversation",
                    files: 1,
                    bytes: 1200n,
                  },
                ],
                retained: ["Project workspace"],
              })
            }
            purge={async () => {
              setPurged(true);
              return true;
            }}
          />
        )}
      >
        <PreviewMetadata agent={agent} busy={sending || active} />
      </ConversationComposer>
    </PreviewFrame>
  );
}
