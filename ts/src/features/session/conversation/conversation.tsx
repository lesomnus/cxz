import { EventView } from "./event-view";
import { QuestionCard } from "#src/features/session/questions/question-card.tsx";
import { ConversationComposer } from "#src/features/session/composer/conversation-composer.tsx";
import { SessionMenu } from "#src/features/session/components/session-menu.tsx";
import { manageSession } from "#src/features/session/model/session-actions.ts";
import { editSession } from "#src/features/session/model/session-edit.ts";
import { useNavigate } from "@tanstack/react-router";
import { t, translateKnown } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useQuery } from "@lesomnus/payday/react";
import { SessionService, SessionRefSchema } from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import {
  SessionHistory,
  type HistoryPage,
  type LoadEventDetails,
} from "#src/features/session/model/session-history.ts";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import {
  mergeEvents,
  isTranscriptEvent,
  payload,
  pendingAfter,
} from "#src/features/session/model/journal.ts";
import { mergeMetadata } from "#src/features/session/model/session-metadata.ts";
import { sessionInfo } from "#src/features/session/model/session-info.ts";
import {
  ModelSettings,
  modelCatalog,
} from "#src/features/session/composer/model-settings.tsx";
import { UsageInfo } from "#src/features/session/composer/usage-info.tsx";
import {
  advanceTurn,
  snapshotTurn,
  type TurnProgress,
} from "#src/features/session/model/turn-progress.ts";
import { sessionCommands } from "#src/features/session/composer/composer-commands.ts";
import { useSendMotion } from "./send-motion";
import {
  WorkspaceTerminal,
  terminalShortcut,
} from "#src/features/workspace/terminal/workspace-terminal.tsx";
import {
  FloatingCardProvider,
  FloatingCardHost,
} from "#src/features/session/cards/floating-card.tsx";
import { composerPrompt } from "#src/features/session/composer/composer-code.ts";
import { key } from "@lesomnus/payday/store";
import { transcriptEvents } from "#src/features/session/model/tool-activity.ts";
import { Transcript } from "./transcript";
import { atScrollBottom } from "./scroll-physics";

type ConversationProps = { c: Connection; id: string; projects: Project[] };

export function Conversation(props: ConversationProps) {
  useLocale();
  return (
    <FloatingCardProvider>
      <ConversationContent {...props} />
    </FloatingCardProvider>
  );
}
function ConversationContent({ c, id, projects }: ConversationProps) {
  useLocale();
  const navigate = useNavigate();
  const [sending, setSending] = useState(false);
  const current = useQuery(SessionService.method.get, {
    ref: ref(id),
    select: { all: true, project: { all: true } },
  });
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [metadata, setMetadata] = useState<SessionEvent[]>([]);
  const catalogRun = useRef("");
  const history = useMemo(() => new SessionHistory(c.sessions, id), [c, id]);
  const newest = useRef({ anchor: 0n, through: 0n });
  const loadDetails = useCallback<LoadEventDetails>(
    (seq, signal) => history.details(seq, signal),
    [history],
  );
  const tension = useRef(0);
  const scrollMotion = useRef(0);
  const composerInput = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLFormElement>(null);
  const [pending, setPending] = useState<SessionEvent[]>([]);
  const [gap, setGap] = useState(false);
  const [status, setStatus] = useState("Connecting…");
  const [executionState, setExecutionState] = useState("");
  const [turn, setTurn] = useState<TurnProgress>({ runId: "", active: false });
  const [error, setError] = useState("");
  const [draft, setDraft] = useState(c.drafts.get(id) ?? "");
  const [busy, setBusy] = useState(false);
  const [terminalVisible, setTerminalVisible] = useState(false);
  const [terminalActivated, setTerminalActivated] = useState(false);
  const terminalVisibleRef = useRef(false);
  function showTerminal(show: boolean) {
    terminalVisibleRef.current = show;
    if (show) setTerminalActivated(true);
    setTerminalVisible(show);
    if (!show)
      composerInput.current
        ?.querySelector<HTMLTextAreaElement>("textarea")
        ?.focus();
  }
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (!terminalShortcut(event)) return;
      event.preventDefault();
      event.stopPropagation();
      if (!event.repeat) showTerminal(!terminalVisibleRef.current);
    };
    document.addEventListener("keydown", keydown, true);
    return () => document.removeEventListener("keydown", keydown, true);
  }, []);
  const [follow, setFollow] = useState(true);
  const [latestShown, setLatestShown] = useState(false);
  const latestTravel = useRef({ shown: false, distance: 0 });
  useEffect(() => {
    if (!follow) return;
    latestTravel.current = { shown: false, distance: 0 };
    setLatestShown(false);
  }, [follow]);
  const pane = useRef<HTMLDivElement>(null);
  const sendMotion = useSendMotion({
    source: composerInput,
    pane,
    events,
    draft,
  });
  const lock = useRef(false);
  const historyRequest = useRef<AbortController | null>(null);
  const [precedingPrompt, setPrecedingPrompt] = useState<{
    before: bigint;
    through: bigint;
    event?: SessionEvent;
  }>();
  const precedingRef = useRef<typeof precedingPrompt>(undefined);
  const [jumpTarget, setJumpTarget] = useState<string>();
  const floor = useRef(0n);
  const latestSeq = useRef(0n);
  const detached = useRef(false);
  const eventsRef = useRef(events);
  eventsRef.current = events;
  const followRef = useRef(follow);
  followRef.current = follow;
  const firstSeq = events[0]?.seq;
  useEffect(() => {
    if (firstSeq === undefined) return;
    const known = precedingRef.current;
    if (known && known.before <= firstSeq && firstSeq <= known.through) return;
    const through =
      events.find((event) => event.kind === "input")?.seq ??
      events.at(-1)!.seq + 1n;
    const remember = (event?: SessionEvent) => {
      const next = { before: firstSeq, through, event };
      precedingRef.current = next;
      setPrecedingPrompt(next);
    };
    const controller = new AbortController();
    // Resolve just the prompt preceding the cached window, retaining one event.
    // This read-only lookup does not prepend rows or change scrollbar geometry.
    void (async () => {
      if (history.mode === "summary") {
        const page = await c.sessions.transcript(
          {
            ref: ref(id),
            beforeSeq: firstSeq,
            limit: 1,
            snapshotSeq: latestSeq.current,
          },
          { signal: controller.signal },
        );
        if (!controller.signal.aborted)
          remember(
            page.events[0]?.kind === "input"
              ? page.events[0]
              : page.precedingInput,
          );
        return;
      }
      let before = firstSeq;
      while (before > 0n && !controller.signal.aborted) {
        const page = await history.page(
          { beforeSeq: before, snapshotSeq: latestSeq.current },
          controller.signal,
        );
        if (controller.signal.aborted) return;
        const prompt = [...page.events]
          .reverse()
          .find((event) => event.seq < before && event.kind === "input");
        if (prompt) {
          remember(prompt);
          return;
        }
        if (
          !page.hasOlder ||
          !page.events.length ||
          page.events[0].seq >= before
        )
          break;
        before = page.events[0].seq;
      }
      if (!controller.signal.aborted) remember();
    })().catch((e) => {
      if (!controller.signal.aborted)
        setError(
          t("Cannot load preceding input: {error}", { error: String(e) }),
        );
    });
    return () => controller.abort();
  }, [c, id, firstSeq, history]);

  async function showPrompt(seq: string) {
    historyRequest.current?.abort();
    const controller = new AbortController();
    historyRequest.current = controller;
    pane.current?.dispatchEvent(new Event("scroll-jump"));
    followRef.current = false;
    setFollow(false);
    try {
      const target = BigInt(seq);
      const page = await history.page(
        { afterSeq: target - 1n, snapshotSeq: latestSeq.current },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      rememberPage(page);
      if (
        !page.events.some(
          (event) => event.seq === target && event.kind === "input",
        )
      ) {
        setError(t("This input is no longer available in retained history."));
        return;
      }
      detached.current = page.hasNewer;
      rememberMetadata(page.events);
      setEvents(page.events);
      setJumpTarget(seq);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(t("Cannot open input: {error}", { error: String(e) }));
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
  }

  function rememberPage(page: HistoryPage) {
    rememberMetadata(page.metadata);
    const first = page.events[0]?.seq;
    const last = page.events.at(-1)?.seq;
    if (first !== undefined) {
      if (!page.hasOlder) floor.current = first - 1n;
      const next = {
        before: first,
        through: page.events.find((e) => e.kind === "input")?.seq ?? last! + 1n,
        event: page.precedingInput,
      };
      precedingRef.current = next;
      setPrecedingPrompt(next);
    }
    if (last !== undefined && !page.hasNewer)
      newest.current = { anchor: last, through: page.snapshotSeq };
  }
  function atLatest(rows: SessionEvent[]) {
    const last = rows.at(-1)?.seq ?? 0n;
    return (
      last >= latestSeq.current ||
      (last >= newest.current.anchor &&
        newest.current.through >= latestSeq.current)
    );
  }
  function rememberMetadata(rows: SessionEvent[]) {
    setMetadata((old) => mergeMetadata(old, rows));
  }
  function updateShadow() {
    const el = pane.current,
      input = composerInput.current;
    const area = el?.parentElement;
    if (!el || !input || !area) return;
    const depth = Math.max(0, el.scrollHeight - el.scrollTop - el.clientHeight);
    const pull = Math.min(1, Math.abs(tension.current) / 20);
    const ramp = (distance: number) => {
      const n = Math.min(1, Math.max(0, distance) / 160);
      return n * n * (3 - 2 * n);
    };
    const bottom = ramp(depth);
    const top =
      eventsRef.current[0]?.seq > floor.current + 1n ? 1 : ramp(el.scrollTop);
    const topPull = Math.min(1, Math.max(0, tension.current) / 20);
    const bottomPull = tension.current > 0 ? 0.75 * pull : pull;
    // Content moves upwards as scrollTop increases. Extend its outgoing fade
    // during ordinary movement; elastic pulls produce a much longer veil.
    const topMotion = Math.max(0, scrollMotion.current);
    const bottomMotion = Math.max(0, -scrollMotion.current);
    area.style.setProperty(
      "--bottom-fade-height",
      `${input.offsetHeight * (bottom + 0.8 * bottomMotion + 3.5 * bottomPull)}px`,
    );
    area.style.setProperty(
      "--bottom-fade-opacity",
      String(Math.max(bottom, pull)),
    );
    area.style.setProperty(
      "--top-fade-height",
      `${input.offsetHeight * (0.45 * top + 0.8 * topMotion + 3.5 * topPull)}px`,
    );
    area.style.setProperty(
      "--top-fade-opacity",
      String(Math.max(top, topPull)),
    );
  }
  useEffect(
    () => () => {
      historyRequest.current?.abort();
    },
    [],
  );
  useEffect(() => {
    c.drafts.set(id, draft);
  }, [c, id, draft]);
  useEffect(() => {
    let canceled = false;
    let active: AbortController | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let cursor = 0n;
    async function connect() {
      active = new AbortController();
      const signal = active.signal;
      setStatus("Connecting…");
      try {
        const snapshot = await c.sessions.get(
          { ref: ref(id), select: { all: true } },
          { signal },
        );
        if (canceled) return;
        setPending(snapshot.status?.pending ?? []);
        const snapshotSeq = snapshot.status?.lastSeq ?? 0n;
        if (snapshotSeq > latestSeq.current) latestSeq.current = snapshotSeq;
        let loaded: SessionEvent[] = [];
        if (cursor === 0n) {
          const page = await history.page({ snapshotSeq }, signal);
          if (canceled) return;
          rememberPage(page);
          loaded = page.events;
          // The stream replays native records, including the ones omitted from
          // the summary. Resume at its snapshot fence, not the last display row.
          cursor = page.snapshotSeq;
        } else {
          // On reconnect, replay every native update after the stream cursor:
          // an old task's completion has an earlier summary row anchor.
          do {
            const page = await history.nativePage(cursor, signal);
            if (canceled) return;
            rememberMetadata(page.events);
            if (cursor > 0n && page.events[0]?.seq > cursor + 1n) setGap(true);
            const next = page.events.at(-1)?.seq ?? cursor;
            if (next <= cursor) break;
            loaded = mergeEvents(loaded, page.events);
            cursor = next;
          } while (cursor < snapshotSeq);
        }
        if (loaded.length) {
          setPending(
            pendingAfter(
              snapshot.status?.pending ?? [],
              loaded,
              snapshot.status?.lastSeq ?? 0n,
            ),
          );
          setEvents((old) =>
            detached.current ? old : mergeEvents(old, loaded),
          );
        }
        setExecutionState(snapshot.status?.state ?? "");
        setTurn((old) => snapshotTurn(snapshot.status, loaded, old));
        setStatus("Live");
        for await (const e of c.sessions.events(
          { ref: ref(id), afterSeq: cursor, clientId: c.clientId },
          { signal },
        )) {
          if (canceled) return;
          if (e.kind === "state") setExecutionState(e.text);
          setTurn((old) => advanceTurn(old, e, true));
          rememberMetadata([e]);
          cursor = e.seq > cursor ? e.seq : cursor;
          if (cursor > latestSeq.current) latestSeq.current = cursor;
          // Receive every native record, but token/protocol traffic cannot
          // change the reading layout. Metadata is reduced separately above.
          if (!isTranscriptEvent(e) && e.kind !== "approval_resolved") {
            if (!detached.current) newest.current.through = cursor;
            continue;
          }
          setEvents((old) => {
            if (detached.current) return old;
            const next = mergeEvents(old, [e]);
            // Protocol compaction alone must not detach a reader. Freeze only
            // when adding live records would evict a visible reading row.
            if (
              !followRef.current &&
              transcriptEvents(old).events.some(
                (row) => row.seq < (next[0]?.seq ?? 0n),
              )
            ) {
              detached.current = true;
              return old;
            }
            newest.current = {
              anchor: next.at(-1)?.seq ?? 0n,
              through: cursor,
            };
            return next;
          });
          setPending((old) => pendingAfter(old, [e]));
        }
      } catch (e) {
        if (canceled) return;
        setStatus(t("Disconnected · retrying: {error}", { error: String(e) }));
      } finally {
        if (!canceled) timer = setTimeout(connect, 2000);
      }
    }
    void connect();
    const resume = () => {
      if (document.visibilityState === "visible") {
        active?.abort();
      }
    };
    const offline = () => active?.abort();
    window.addEventListener("offline", offline);
    document.addEventListener("visibilitychange", resume);
    return () => {
      canceled = true;
      active?.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("offline", offline);
    };
  }, [c, id, history]);
  async function loadHistory(direction: "older" | "newer", goLatest = false) {
    if (goLatest) pane.current?.dispatchEvent(new Event("scroll-jump"));
    if (historyRequest.current) {
      if (!goLatest) return;
      historyRequest.current.abort();
      historyRequest.current = null;
    }
    const old = eventsRef.current;
    if (!old.length) return;
    const first = old[0].seq,
      last = old.at(-1)!.seq;
    if (
      !goLatest &&
      (direction === "older" ? first <= floor.current + 1n : atLatest(old))
    )
      return;
    if (goLatest && atLatest(old)) {
      detached.current = false;
      setFollow(true);
      if (pane.current) pane.current.scrollTop = pane.current.scrollHeight;
      return;
    }
    const controller = new AbortController();
    historyRequest.current = controller;
    try {
      const page = await history.page(
        goLatest
          ? { snapshotSeq: latestSeq.current }
          : direction === "older"
            ? { beforeSeq: first, snapshotSeq: latestSeq.current }
            : { afterSeq: last, snapshotSeq: latestSeq.current },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      rememberPage(page);
      const incoming = page.events;
      if (!incoming.length) {
        if (direction === "older") {
          floor.current = first - 1n;
          updateShadow();
        }
        return;
      }
      rememberMetadata(incoming);
      const next = goLatest
        ? incoming
        : mergeEvents(eventsRef.current, incoming, direction);
      detached.current = !atLatest(next);
      setEvents(next);
      if (goLatest) setFollow(true);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(t("Cannot load history: {error}", { error: String(e) }));
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
  }
  const transcript = useMemo(() => transcriptEvents(events), [events]);
  const s = current.data;
  const sessionProject =
    s?.project?.id &&
    projects.find((project) => key(project.id) === key(s.project!.id));
  const completions = transcript.completions;
  const combined = useMemo(
    () =>
      [
        ...new Map([...events, ...metadata].map((e) => [e.seq, e])).values(),
      ].sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0)),
    [events, metadata],
  );
  const info = useMemo(() => sessionInfo(s, combined), [s, combined]);
  const catalog = useMemo(() => modelCatalog(s, combined), [s, combined]);
  // Model choices have a dedicated projection; avoid scanning the journal.
  useEffect(() => {
    const run = s?.status?.runId;
    if (!run || !events.length || catalog || catalogRun.current === run) return;
    catalogRun.current = run;
    const controller = new AbortController();
    void c.sessions
      .models({ ref: ref(id) }, { signal: controller.signal })
      .then((page) => {
        if (!controller.signal.aborted && page.data.length)
          rememberMetadata([
            create(SessionEventSchema, {
              seq: page.catalogSeq,
              timeMs: page.catalogMs,
              runId: page.runId,
              kind: "models",
              payload: page.data,
            }),
          ]);
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setError(
            t("Cannot read model choices: {error}", { error: String(error) }),
          );
      });
    return () => controller.abort();
  }, [c, id, s?.status?.runId, events.length > 0]);
  useEffect(() => {
    if (!composerInput.current) return;
    const resize = new ResizeObserver(updateShadow);
    resize.observe(composerInput.current);
    return () => resize.disconnect();
  }, []);
  async function changeSetting(kind: "model" | "effort", value: string) {
    if (!catalog || s?.status?.state !== "idle") {
      setError(
        t(
          !catalog
            ? "Provider choices not reported"
            : "Settings require an idle session",
        ),
      );
      return false;
    }
    return action(async (session) => {
      async function apply(name: "model" | "effort", choice: string) {
        const command = control(session);
        let afterSeq = latestSeq.current;
        const receipt = await c.sessions.send({
          ...command,
          text: `/${name} ${choice}`,
        });
        if (receipt.status === "accepted") return;
        if (receipt.status === "rejected")
          throw new Error(t("Provider rejected the setting"));
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 20000);
        try {
          while (!controller.signal.aborted) {
            const page = await c.sessions.history(
              { ref: ref(id), afterSeq },
              { signal: controller.signal },
            );
            rememberMetadata(page.events);
            for (const event of page.events) {
              if (event.requestId !== command.clientId) continue;
              if (event.kind === "setting" && event.text === name) return;
              if (
                event.kind === "setting_status" &&
                event.text.startsWith("rejected")
              )
                throw new Error(event.text);
              if (
                event.kind === "receipt" &&
                payload(event).status === "rejected"
              )
                throw new Error(t("Provider rejected the setting"));
            }
            afterSeq = page.events.at(-1)?.seq ?? afterSeq;
            await new Promise((resolve) => setTimeout(resolve, 100));
          }
          throw new Error(t("Provider has not confirmed the setting"));
        } finally {
          clearTimeout(timeout);
        }
      }
      // The runtime requires clearing an explicit effort before changing models.
      if (kind === "model" && catalog.effort) await apply("effort", "default");
      await apply(kind, value);
    });
  }
  async function action(fn: (s: Session) => Promise<unknown>) {
    if (lock.current || !s) return false;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      await fn(s);
      return true;
    } catch (e) {
      setError(
        t(
          "{error}. The result may be unknown after a disconnect; inspect the conversation before retrying.",
          { error: String(e) },
        ),
      );
      return false;
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  function control(s: Session) {
    return {
      ref: ref(id),
      runId: s.status?.runId,
      clientId: crypto.randomUUID(),
    };
  }
  async function send(e: React.FormEvent) {
    e.preventDefault();
    if (!draft.trim()) return;
    const sent = draft;
    const clearCommand = () => {
      if (c.drafts.get(id) === sent) c.drafts.set(id, "");
      setDraft((old) => (old === sent ? "" : old));
    };
    const setting = /^\/(model|effort)(?:[ \t]+(\S+))?[ \t]*$/.exec(sent);
    if (setting) {
      const kind = setting[1] as "model" | "effort";
      if (!setting[2]) {
        const trigger = document.querySelector<HTMLButtonElement>(
          `.${kind}-field .setting-trigger`,
        );
        if (trigger && !trigger.disabled) {
          clearCommand();
          trigger.click();
        } else setError(t("Settings require an idle session"));
      } else if (!lock.current) {
        setSending(true);
        try {
          if (await changeSetting(kind, setting[2])) clearCommand();
        } finally {
          setSending(false);
        }
      }
      return;
    }
    await action(async (s) => {
      const text = composerPrompt(sent, c.pastes);
      const motion = sendMotion.prepare(text, latestSeq.current);
      try {
        setSending(true);
        try {
          const receipt = await c.sessions.send({ ...control(s), text });
          if (receipt.status === "rejected")
            throw new Error(t("Provider rejected the input"));
        } finally {
          setSending(false);
        }
        // A session switch can unmount this view during the decorative departure.
        if (c.drafts.get(id) === sent) c.drafts.set(id, "");
        await sendMotion.depart(motion);
        setDraft((old) => (old === sent ? "" : old));
        if (detached.current) await loadHistory("newer", true);
        pane.current?.dispatchEvent(
          new CustomEvent("scroll-jump", { detail: "send" }),
        );
        followRef.current = true;
        setFollow(true);
        if (pane.current) pane.current.scrollTop = pane.current.scrollHeight;
        sendMotion.enter(motion);
      } catch (error) {
        if (motion) sendMotion.cancel(motion);
        throw error;
      }
    });
  }
  async function reply(e: SessionEvent, allow: boolean, answersJson = "") {
    await action(async () => {
      await c.sessions.reply({
        ref: ref(id),
        runId: e.runId,
        clientId: crypto.randomUUID(),
        requestId: e.requestId,
        allow,
        answersJson,
      });
      setPending((old) =>
        old.filter((p) => p.requestId !== e.requestId || p.runId !== e.runId),
      );
    });
  }
  return (
    <main
      className="conversation"
      aria-description={`${s?.status?.state ?? ""} · ${translateKnown(status)}`}
    >
      <Transcript
        pane={pane}
        events={transcript.events}
        follow={followRef}
        precedingPrompt={
          firstSeq !== undefined &&
          precedingPrompt &&
          precedingPrompt.before <= firstSeq &&
          firstSeq <= precedingPrompt.through
            ? precedingPrompt.event
            : undefined
        }
        jumpTarget={jumpTarget}
        onPromptJump={(seq) => void showPrompt(seq)}
        onJumped={() => setJumpTarget(undefined)}
        onNavigate={() => {
          followRef.current = false;
          setFollow(false);
        }}
        render={(e) => (
          <EventView
            e={e}
            agent={s?.agent ?? ""}
            completion={completions.get(e.seq.toString())}
            activity={transcript.activities.get(e.seq)}
            loadDetails={history.mode === "summary" ? loadDetails : undefined}
          />
        )}
        notice={
          gap && (
            <p className="muted history-note">
              {t("Earlier history is no longer available.")}
            </p>
          )
        }
        onReadingMove={(delta) => {
          if (followRef.current) return;
          const travel = latestTravel.current;
          // Measure from the furthest position in the opposite direction. Small
          // reversals consume the accumulated distance instead of toggling UI.
          travel.distance = Math.max(
            0,
            travel.distance + (travel.shown ? delta : -delta),
          );
          if (travel.distance < 96) return;
          travel.shown = !travel.shown;
          travel.distance = 0;
          setLatestShown(travel.shown);
        }}
        onTension={(stretch) => {
          tension.current = stretch;
          updateShadow();
        }}
        onMotion={(motion) => {
          scrollMotion.current = motion;
          updateShadow();
        }}
        older={() => void loadHistory("older")}
        newer={() => void loadHistory("newer")}
        onScroll={(reading) => {
          const el = pane.current!;
          updateShadow();
          const next = !reading && !detached.current && atScrollBottom(el);
          followRef.current = next;
          setFollow(next);
        }}
      />
      <FloatingCardHost>
        {pending.map((e) => (
          <QuestionCard
            key={`${e.runId}:${e.requestId}`}
            e={e}
            agent={s?.agent ?? ""}
            pastes={c.pastes}
            busy={busy}
            reply={reply}
          />
        ))}
      </FloatingCardHost>
      {!!(error || current.error) && (
        <p className="error" role="alert">
          {error || String(current.error)}
        </p>
      )}
      <ConversationComposer
        formRef={composer}
        inputRef={composerInput}
        draft={draft}
        pastes={c.pastes}
        onChange={setDraft}
        onSubmit={send}
        commands={sessionCommands(s?.agent ?? "", catalog)}
        canSend={!busy && !!s && !!draft.trim()}
        sending={sending}
        busy={busy}
        turn={turn}
        working={turn.active && ["working", "running"].includes(executionState)}
        interrupt={() =>
          void action(async (session) => {
            const receipt = await c.sessions.interrupt({
              ...control(session),
              runId: turn.runId,
            });
            if (receipt.status === "rejected")
              throw new Error(t("Provider rejected the interrupt"));
          })
        }
        latestVisible={!follow && latestShown}
        onLatest={() => void loadHistory("newer", true)}
        terminalVisible={terminalVisible}
        terminalAvailable={!!s?.project?.id.length}
        onTerminal={() => showTerminal(!terminalVisibleRef.current)}
        menu={
          <SessionMenu
            session={s && { ...s, project: sessionProject ?? s.project }}
            info={info}
            busy={busy}
            edit={async (field, value) => {
              const updated = await editSession(c.sessions, id, field, value);
              c.store.apply("cxz.Session", [
                { id: updated.id, value: updated },
              ]);
              return updated;
            }}
            manage={(operation, run) =>
              action(async () => {
                const updated = await manageSession(
                  c.sessions,
                  create(SessionRefSchema, ref(id)),
                  operation,
                  run,
                );
                c.store.apply("cxz.Session", [
                  { id: updated.id, value: updated },
                ]);
              })
            }
            previewPurge={async () => {
              let plan;
              await action(async () => {
                plan = await c.sessions.purge(
                  { ref: ref(id), dryRun: true },
                  { timeoutMs: 20_000 },
                );
              });
              return plan;
            }}
            purge={() =>
              action(async (session) => {
                await c.sessions.purge(
                  { ref: ref(id) },
                  { timeoutMs: 120_000 },
                );
                c.store.apply("cxz.Session", [{ id: session.id }]);
                c.drafts.delete(id);
                await navigate({ to: "/sessions", replace: true });
              })
            }
          />
        }
      >
        <ModelSettings
          session={s}
          info={info}
          catalog={catalog}
          busy={busy}
          change={(kind, value) => void changeSetting(kind, value)}
        />
        <UsageInfo info={info} />
      </ConversationComposer>
      {terminalActivated && !!s?.project?.id.length && (
        <WorkspaceTerminal
          c={c}
          projectId={s.project.id}
          visible={terminalVisible}
          hide={() => showTerminal(false)}
        />
      )}
    </main>
  );
}
