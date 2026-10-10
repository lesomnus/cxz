import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../button";
import { ConversationComposer } from "../conversation-composer";
import { FloatingCardHost } from "../floating-card";
import { composerPrompt } from "../composer-code";
import { sessionCommands } from "../composer-commands";
import { useSendMotion } from "../send-motion";
import { atScrollBottom } from "../scroll-physics";
import type { TurnProgress } from "../turn-progress";
import {
  conversationEvents,
  burstEvent,
  responseEvent,
  storyEvent,
  toolEvents,
} from "./fixtures";
import {
  idleTurn,
  PreviewFrame,
  PreviewMetadata,
  PreviewTranscript,
  previewPastes,
} from "./conversation-preview";

export function ConversationPlayground({
  agent = "codex",
  acknowledgementMs = 450,
  responseMs = 1800,
  historyTurns = 1,
  burstUpdates = 0,
  burstIntervalMs = 40,
}: {
  agent?: string;
  acknowledgementMs?: number;
  responseMs?: number;
  historyTurns?: number;
  burstUpdates?: number;
  burstIntervalMs?: number;
}) {
  const [reset, setReset] = useState(0);
  return (
    <div className="storybook-playground">
      <header>
        <span>Local simulation · no server or account required</span>
        <Button onClick={() => setReset((n) => n + 1)}>Reset preview</Button>
      </header>
      <PreviewFrame key={`${reset}/${agent}/${historyTurns}`}>
        <ConversationSimulation
          agent={agent}
          acknowledgementMs={acknowledgementMs}
          responseMs={responseMs}
          historyTurns={historyTurns}
          burstUpdates={burstUpdates}
          burstIntervalMs={burstIntervalMs}
        />
      </PreviewFrame>
    </div>
  );
}

// Delays demonstrate receipt/working states, not a replacement RPC implementation.
function ConversationSimulation({
  agent,
  acknowledgementMs,
  responseMs,
  historyTurns,
  burstUpdates,
  burstIntervalMs,
}: Required<Parameters<typeof ConversationPlayground>[0]>) {
  const [events, setEvents] = useState(() =>
    historyTurns === 1
      ? [
          storyEvent(1, "input", "Explore the conversation UI."),
          ...toolEvents(),
          responseEvent(5, agent),
        ]
      : conversationEvents(agent, historyTurns),
  );
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [turn, setTurn] = useState<TurnProgress>(idleTurn);
  const [reading, setReading] = useState(false);
  const [received, setReceived] = useState(0);
  const bottomGap = useRef<HTMLOutputElement>(null);
  const follow = useRef(true);
  const pane = useRef<HTMLDivElement>(null);
  const source = useRef<HTMLDivElement>(null);
  const pastes = useRef(previewPastes());
  const request = useRef<AbortController | undefined>(undefined);
  const mounted = useRef(true);
  const sequence = useRef(events.at(-1)?.seq ?? 0n);
  const motion = useSendMotion({ source, pane, events, draft });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      request.current?.abort();
    };
  }, []);

  function delay(ms: number, signal: AbortSignal) {
    return new Promise<void>((resolve) => {
      if (signal.aborted) return resolve();
      const done = () => {
        clearTimeout(timer);
        signal.removeEventListener("abort", done);
        resolve();
      };
      const timer = window.setTimeout(done, ms);
      signal.addEventListener("abort", done, { once: true });
    });
  }
  function latest() {
    follow.current = true;
    setReading(false);
    pane.current?.dispatchEvent(
      new CustomEvent("scroll-jump", { detail: "send" }),
    );
    if (pane.current) pane.current.scrollTop = pane.current.scrollHeight;
  }
  function interrupt() {
    request.current?.abort();
    request.current = undefined;
    setTurn(idleTurn);
  }
  async function send(event: FormEvent) {
    event.preventDefault();
    if (sending || turn.active || !draft.trim()) return;
    const submitted = draft;
    setReceived(0);
    const text = composerPrompt(submitted, pastes.current);
    const controller = new AbortController();
    request.current = controller;
    const departure = motion.prepare(text, sequence.current);
    setSending(true);
    await delay(acknowledgementMs, controller.signal);
    if (controller.signal.aborted) return;
    const input = storyEvent(Number(++sequence.current), "input", text);
    input.timeMs = BigInt(Date.now());
    setEvents((old) => [...old, input]);
    setTurn({
      runId: "storybook",
      active: true,
      startedAt: Date.now(),
      key: `storybook/${input.seq}`,
    });
    setSending(false);
    await motion.depart(departure);
    // Stopping a confirmed turn still lets its accepted draft finish departing.
    if (!mounted.current) return;
    setDraft((old) => (old === submitted ? "" : old));
    latest();
    motion.enter(departure);
    await delay(responseMs, controller.signal);
    if (controller.signal.aborted) return;
    for (let index = 0; index < burstUpdates; index++) {
      const update = burstEvent(
        index,
        Number(++sequence.current),
        agent,
        `burst-${input.seq}`,
      );
      update.timeMs = BigInt(Date.now());
      setEvents((old) => [...old, update]);
      setReceived(index + 1);
      await delay(burstIntervalMs, controller.signal);
      if (controller.signal.aborted) return;
    }
    const answer = responseEvent(
      Number(++sequence.current),
      agent,
      "Message received in the **Storybook preview**.\n\n" +
        "This simulated response uses the same card, model snapshot and metrics UI as a live session.\n\n" +
        "Try another message, a code block, or a multiline paste.",
      acknowledgementMs + responseMs,
    );
    answer.timeMs = BigInt(Date.now());
    setEvents((old) => [...old, answer]);
    setTurn(idleTurn);
    request.current = undefined;
  }
  return (
    <>
      {burstUpdates > 0 && (
        <div
          className="storybook-burst-status"
          aria-label="Stream preview status"
        >
          <span data-following={!reading}>
            {reading ? "Reading history" : "Following latest"}
          </span>
          <span>
            Updates {received} / {burstUpdates}
          </span>
          <span>
            Bottom gap <output ref={bottomGap}>0</output>px
          </span>
        </div>
      )}
      <PreviewTranscript
        events={events}
        agent={agent}
        pane={pane}
        follow={follow}
        navigate={() => {
          follow.current = false;
        }}
        reading={(active) => {
          const el = pane.current;
          if (!el) return;
          const gap = Math.max(
            0,
            el.scrollHeight - el.scrollTop - el.clientHeight,
          );
          // Use the application's follow decision, including wheel/drag frames.
          // The old preview only showed gesture activity and never resumed follow.
          follow.current = !active && atScrollBottom(el);
          setReading(!follow.current);
          if (bottomGap.current)
            bottomGap.current.value = String(Math.round(gap));
        }}
      />
      <FloatingCardHost />
      <ConversationComposer
        inputRef={source}
        draft={draft}
        pastes={pastes.current}
        onChange={setDraft}
        onSubmit={(e) => void send(e)}
        commands={sessionCommands(agent)}
        sending={sending}
        busy={sending}
        canSend={!sending && !turn.active && !!draft.trim()}
        turn={turn}
        working={turn.active}
        interrupt={interrupt}
        latestVisible={reading}
        onLatest={latest}
      >
        <PreviewMetadata agent={agent} busy={sending || turn.active} />
      </ConversationComposer>
    </>
  );
}
