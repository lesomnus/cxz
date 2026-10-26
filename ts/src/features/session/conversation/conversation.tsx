import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import {
  projectId as projectUUID,
  projectRef,
} from "#src/shared/api/project-id.ts";
import { EventView } from "./event-view";
import {
  eventSource,
  snapshotSource,
  eventListSource,
} from "#src/features/session/model/event-source.ts";
import errorStyles from "./activity-card.module.css";
import { attachSummaries } from "#src/features/session/model/session-aux.ts";
import { useSessionAux } from "#src/features/session/model/use-session-aux.ts";
import { QuestionCard } from "#src/features/session/questions/question-card.tsx";
import { requiredSessionLogin } from "#src/features/accounts/login-required.ts";
import { ConversationComposer } from "#src/features/session/composer/conversation-composer.tsx";
import {
  downloadArgument,
  type LookupPaths,
} from "#src/features/session/composer/composer-paths.ts";
import {
  containerDownload,
  saveContainerDownload,
} from "#src/features/session/composer/composer-download.ts";
import { SessionMenu } from "#src/features/session/components/session-menu.tsx";
import { Button } from "@lesomnus/nox";
import { Code, ConnectError } from "@connectrpc/connect";
import { authenticationRequired } from "#src/features/session/model/session-auth.ts";
import {
  manageSession,
  prepareSessionLogin,
  sessionLoginSnapshot,
} from "#src/features/session/model/session-actions.ts";
import { editSession } from "#src/features/session/model/session-edit.ts";
import { useNavigate } from "@tanstack/react-router";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { t, translateKnown } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useQuery } from "@lesomnus/payday/react";
import { SessionService, SessionRefSchema } from "#gen/cxz/session_svc_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import {
  type HistoryPage,
  type LoadEventDetails,
} from "#src/features/session/model/session-history.ts";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { mergeEvents, payload } from "#src/features/session/model/journal.ts";
import { mergeMetadata } from "#src/features/session/model/session-metadata.ts";
import { sessionInfo } from "#src/features/session/model/session-info.ts";
import {
  ModelSettings,
  modelCatalog,
} from "#src/features/session/composer/model-settings.tsx";
import { UsageInfo } from "#src/features/session/composer/usage-info.tsx";

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
import { transcriptRows } from "#src/features/session/model/tool-activity.ts";
import { Transcript } from "./transcript";
import { ConversationLoading } from "./conversation-loading";
import { atScrollBottom } from "./scroll-physics";
import { uploadFile } from "#src/features/session/composer/composer-upload.ts";
import { attachmentsReady } from "#src/features/session/composer/composer-pastes.ts";
import { QueuedMessage } from "#src/features/session/components/queued-message.tsx";
import { AuxiliaryControls } from "#src/features/auxiliary/auxiliary-controls.tsx";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import { BackgroundReport } from "#src/features/activity/background-report.tsx";
import { useSessionBackground } from "#src/features/session/model/use-session-background.ts";
import { useVisibleApprovals } from "#src/features/session/model/use-visible-approvals.ts";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { LoginPanel } from "#src/features/accounts/login-panel.tsx";
import { useFloatingCard } from "#src/features/session/cards/floating-card.tsx";
import { WorkspaceContext } from "#src/app/workspace-context.tsx";
import { SecretInput } from "#src/features/session/composer/secret-input.tsx";
import { pasteRanges } from "#src/features/session/composer/composer-pastes.ts";
import { SessionSearch } from "./session-search";

type ConversationProps = { c: Connection; id: string; projects: Project[] };

export function Conversation(props: ConversationProps) {
  useLocale();
  return (
    <FloatingCardProvider key={props.id}>
      <ConversationContent {...props} />
    </FloatingCardProvider>
  );
}
function ConversationContent({ c, id, projects }: ConversationProps) {
  const conversationArea = useRef<HTMLElement>(null);
  useLocale();
  const navigate = useNavigate();
  const openCard = useFloatingCard();
  const workspaceContext = useContext(WorkspaceContext);
  const feed = useMemo(() => c.conversations.get(id), [c, id]);
  const cached = useSyncExternalStore(feed.subscribe, feed.snapshot);
  const {
    events,
    metadata,
    pending,
    gap,
    status,
    loading: initialLoad,
    executionState,
    turn,
    queued,
  } = cached;
  const { setEvents, setMetadata, setPending, setQueued } = feed;
  const { seq: requestedSeq } = useWorkspaceRoute();
  const openedSearch = useRef("");
  const [sending, setSending] = useState(false);
  const current = useQuery(SessionService.method.get, {
    ref: ref(id),
    select: { all: true, project: { all: true }, account: { all: true } },
  });
  const catalogRun = useRef("");
  const history = feed.history;
  const newest = feed.newest;
  const loadDetails = useCallback<LoadEventDetails>(
    (seq, signal) => history.details(seq, signal),
    [history],
  );
  const tension = useRef(0);
  const scrollMotion = useRef(0);
  const composerInput = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLFormElement>(null);
  const downloadRequest = useRef<AbortController | null>(null);
  useEffect(() => () => downloadRequest.current?.abort(), []);
  const visibleApprovals = useVisibleApprovals(pending);
  const background = useSessionBackground(
    c,
    current.data?.id,
    turn.runId || current.data?.status?.runId || "",
  );
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
  const [follow, setFollow] = useState(feed.follow.current);
  const [latestShown, setLatestShown] = useState(
    feed.latestTravel.current.shown,
  );
  const latestTravel = feed.latestTravel;
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
  const [precedingPrompt, setPrecedingPrompt] = useState<
    | {
        before: bigint;
        through: bigint;
        event?: SessionEvent;
      }
    | undefined
  >(feed.preceding);
  const precedingRef = useRef<typeof precedingPrompt>(feed.preceding);
  const [jumpTarget, setJumpTarget] = useState<string>();
  const [searchMatches, setSearchMatches] = useState<readonly string[]>([]);
  const jumpInset = useRef(18);
  const floor = feed.floor;
  const latestSeq = feed.latestSeq;
  const detached = feed.detached;
  const eventsRef = useRef(events);
  eventsRef.current = events;
  const followRef = feed.follow;
  followRef.current = follow;
  const firstSeq = events[0]?.seq;
  useEffect(() => {
    const acknowledge = () => {
      c.attention.setReading(
        follow &&
          status === "Live" &&
          !document.activeElement?.closest(".workspace-terminal") &&
          document.visibilityState === "visible" &&
          document.hasFocus()
          ? id
          : "",
        latestSeq.current,
      );
    };
    acknowledge();
    document.addEventListener("visibilitychange", acknowledge);
    window.addEventListener("focus", acknowledge);
    window.addEventListener("blur", acknowledge);
    document.addEventListener("focusin", acknowledge);
    return () => {
      c.attention.setReading("");
      document.removeEventListener("visibilitychange", acknowledge);
      window.removeEventListener("focus", acknowledge);
      window.removeEventListener("blur", acknowledge);
      document.removeEventListener("focusin", acknowledge);
    };
  }, [c, id, follow, status, events, turn, terminalVisible]);
  useEffect(() => {
    if (firstSeq === undefined) return;
    const known = feed.preceding ?? precedingRef.current;
    if (known && known.before <= firstSeq && firstSeq <= known.through) {
      precedingRef.current = known;
      setPrecedingPrompt(known);
      return;
    }
    const through =
      events.find((event) => event.kind === "input")?.seq ??
      events.at(-1)!.seq + 1n;
    const remember = (event?: SessionEvent) => {
      const next = { before: firstSeq, through, event };
      feed.preceding = next;
      precedingRef.current = next;
      setPrecedingPrompt(next);
    };
    const controller = new AbortController();
    // Resolve just the prompt preceding the cached window, retaining one event.
    // This read-only lookup does not prepend rows or change scrollbar geometry.
    void (async () => {
      {
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

  async function showPrompt(seq: string, inset = 18, matchVisible = false) {
    historyRequest.current?.abort();
    const controller = new AbortController();
    historyRequest.current = controller;
    pane.current?.dispatchEvent(new Event("scroll-jump"));
    followRef.current = false;
    setFollow(false);
    if (events.some((event) => event.seq.toString() === seq)) {
      jumpInset.current = inset;
      if (!matchVisible) setJumpTarget(seq);
      historyRequest.current = null;
      return;
    }
    try {
      const target = BigInt(seq);
      const page = await history.page(
        { afterSeq: target - 1n, snapshotSeq: latestSeq.current },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      rememberPage(page);
      if (!page.events.some((event) => event.seq === target)) {
        setError(t("This message is no longer available in retained history."));
        return;
      }
      detached.current = page.hasNewer;
      rememberMetadata(page.events);
      setEvents(page.events);
      jumpInset.current = inset;
      setJumpTarget(seq);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(t("Cannot open message: {error}", { error: String(e) }));
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
  }

  useEffect(() => {
    if (
      !requestedSeq ||
      status !== "Live" ||
      openedSearch.current === requestedSeq
    )
      return;
    openedSearch.current = requestedSeq;
    void showPrompt(requestedSeq);
  }, [requestedSeq, status]);

  function rememberPage(page: HistoryPage) {
    feed.rememberPage(page);
    precedingRef.current = feed.preceding;
    setPrecedingPrompt(feed.preceding);
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
  useEffect(() => feed.onEvent(background.receive), [feed, background.receive]);
  useEffect(
    () => feed.onReconnect(background.refresh),
    [feed, background.refresh],
  );
  async function loadHistory(direction: "older" | "newer", goLatest = false) {
    if (goLatest) pane.current?.dispatchEvent(new Event("scroll-jump"));
    if (historyRequest.current) {
      if (!goLatest) return;
      historyRequest.current.abort();
      historyRequest.current = null;
    }
    const old = feed.snapshot().events;
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
      const page =
        (goLatest ? feed.latestPage() : undefined) ??
        (await history.page(
          goLatest
            ? { snapshotSeq: latestSeq.current }
            : direction === "older"
              ? { beforeSeq: first, snapshotSeq: latestSeq.current }
              : { afterSeq: last, snapshotSeq: latestSeq.current },
          controller.signal,
        ));
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
        : mergeEvents(feed.snapshot().events, incoming, direction);
      detached.current = !atLatest(next);
      setEvents(next, direction);
      if (goLatest) setFollow(true);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(t("Cannot load history: {error}", { error: String(e) }));
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
  }
  const transcript = useMemo(() => transcriptRows(events), [events]);
  const aux = useSessionAux(c, id);
  const s = current.data;
  const sessionProject =
    s?.project?.id &&
    projects.find((project) => key(project.id) === key(s.project!.id));
  const projectId = projectUUID(sessionProject) ?? "";
  const lookupPaths = useCallback<LookupPaths>(
    async (path, signal) => {
      const entries = [];
      for await (const reply of c.projects.paths(
        { ref: projectRef(projectId), path },
        { signal, timeoutMs: 5000 },
      )) {
        entries.push(...reply.entries);
        if (entries.length >= 2048) break;
      }
      return entries.slice(0, 2048);
    },
    [c, projectId],
  );
  const completions = transcript.completions;
  // Summaries arrive on their own stream and are matched to the turn each one
  // names, not merged into the journal: they are not events.
  const summaries = useMemo(
    () => attachSummaries(transcript.events, aux),
    [transcript.events, aux],
  );
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
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  async function openSessionLogin(run: string) {
    const stopped = await prepareSessionLogin(
      c.sessions,
      create(SessionRefSchema, ref(id)),
      run,
    );
    c.store.apply("cxz.Session", [{ id: stopped.id, value: stopped }]);
    const account = stopped.account?.alias ?? "";
    const loginProject = projectUUID(stopped.project) || projectId;
    if (
      stopped.agent !== "claude" ||
      !account ||
      !loginProject ||
      !stopped.clientId
    )
      throw new Error(t("Session login is unavailable."));
    if (!mounted.current) return;
    let canceled = false;
    openCard({
      title: () => t("Claude account login"),
      onClose: () => {
        canceled = true;
      },
      content: (close) => (
        <LoginPanel
          c={c}
          target={{ account, project: loginProject, key: stopped.clientId }}
          complete={() => {
            if (canceled || !mounted.current) return;
            canceled = true;
            close();
            void action(async () => {
              const updated = await manageSession(
                c.sessions,
                create(SessionRefSchema, ref(id)),
                "resume",
                run,
              );
              c.store.apply("cxz.Session", [
                { id: updated.id, value: updated },
              ]);
            });
          }}
          cancel={() => {
            canceled = true;
            close();
          }}
        />
      ),
    });
  }
  const needsLogin =
    s?.agent === "claude" &&
    authenticationRequired(metadata, s.status?.runId ?? "");
  const forkRequest = useRef<
    { source: string; run: string; clientId: string } | undefined
  >(undefined);
  async function forkConversation(afterLogin = false) {
    return action(async (session) => {
      const run = session.status?.runId ?? "";
      if (forkRequest.current?.source !== id || forkRequest.current.run !== run)
        forkRequest.current = {
          source: id,
          run,
          clientId: crypto.randomUUID(),
        };
      const clientId = forkRequest.current.clientId;
      try {
        const child = await c.sessions.fork(
          { ref: ref(id), runId: run, clientId },
          { timeoutMs: 120000 },
        );
        c.store.apply("cxz.Session", [{ id: child.id, value: child }]);
        await navigate({
          ...sessionLocation(child),
        });
        forkRequest.current = undefined;
      } catch (error) {
        const account = session.account?.alias ?? "";
        if (
          afterLogin ||
          session.agent !== "claude" ||
          !account ||
          !projectId ||
          !requiredSessionLogin(error, account, clientId)
        )
          throw error;
        openCard({
          title: "Session login",
          content: (close) => (
            <LoginPanel
              c={c}
              target={{ account, project: projectId, key: clientId }}
              complete={() => {
                close();
                void forkConversation(true);
              }}
              cancel={close}
            />
          ),
        });
      }
    });
  }
  function control(s: Session) {
    return {
      ref: ref(id),
      runId: s.status?.runId,
      clientId: crypto.randomUUID(),
    };
  }
  function localOpen(kind: "memory" | "usage" | "background" | "aux") {
    if (kind === "memory")
      openCard({
        title: "Shared memory",
        content: () => <MemoryLibrary c={c} session={id} />,
      });
    if (kind === "usage")
      openCard({
        title: "Conversation usage",
        content: () => <UsageReport c={c} session={id} />,
      });
    if (kind === "background")
      openCard({
        title: "Background tasks",
        content: () => (
          <BackgroundReport
            c={c}
            session={s?.id}
            run={turn.runId || s?.status?.runId || ""}
          />
        ),
      });
    if (kind === "aux")
      openCard({
        title: "Auxiliary AI",
        content: () => (
          <AuxiliaryControls
            c={c}
            session={id}
            insert={(text) => {
              setDraft(text);
              c.drafts.set(id, text);
            }}
          />
        ),
      });
  }
  function openSecret(command = false, range?: { start: number; end: number }) {
    if (!s?.status?.runId || !projectId) return;
    const base = command ? "" : draft;
    const textarea =
      composerInput.current?.querySelector<HTMLTextAreaElement>("textarea");
    const start = command
      ? 0
      : (range?.start ?? textarea?.selectionStart ?? base.length);
    const end = command ? 0 : (range?.end ?? textarea?.selectionEnd ?? start);
    const run = s.status.runId;
    openCard({
      title: "Secret",
      content: (close) => (
        <SecretInput
          c={c}
          project={projectId}
          session={id}
          run={run}
          close={close}
          insert={(chip) => {
            if ((c.drafts.get(id) ?? "") !== base)
              throw new Error(
                "Draft changed. Close this card and insert the secret again.",
              );
            c.pastes.set(chip.token, chip);
            const next = base.slice(0, start) + chip.token + base.slice(end);
            setDraft(next);
            c.drafts.set(id, next);
          }}
        />
      ),
    });
  }
  async function send(e: React.FormEvent) {
    e.preventDefault();
    if (!draft.trim()) return;
    if (!attachmentsReady(draft, c.pastes)) {
      setError(t("Wait for uploads to finish, or remove the failed chip."));
      return;
    }
    const sent = draft;
    if (sent.trim() === "/@redact") {
      setDraft("");
      c.drafts.set(id, "");
      openSecret(true);
      return;
    }
    const clearCommand = () => {
      if (c.drafts.get(id) === sent) c.drafts.set(id, "");
      setDraft((old) => (old === sent ? "" : old));
    };
    const localCommands: Record<string, () => void> = {
      "/memory": () => localOpen("memory"),
      "/usage": () => localOpen("usage"),
      "/background": () => localOpen("background"),
      "/aux": () => localOpen("aux"),
    };
    if (localCommands[sent.trim()]) {
      clearCommand();
      localCommands[sent.trim()]();
      return;
    }
    let downloadPath: string | undefined;
    try {
      downloadPath = downloadArgument(sent);
    } catch (error) {
      setError(String(error));
      return;
    }
    if (downloadPath !== undefined) {
      if (lock.current) return;
      if (!projectId) {
        setError(t("Project workspace unavailable"));
        return;
      }
      const controller = new AbortController();
      downloadRequest.current = controller;
      lock.current = true;
      setSending(true);
      setBusy(true);
      setError("");
      try {
        const blob = await containerDownload(
          c.projects,
          projectId,
          downloadPath,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        saveContainerDownload(blob, downloadPath);
        clearCommand();
      } catch (error) {
        if (!controller.signal.aborted) setError(String(error));
      } finally {
        downloadRequest.current = null;
        lock.current = false;
        setSending(false);
        setBusy(false);
      }
      return;
    }
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
      if (
        pasteRanges(sent, c.pastes).some(
          ({ paste }) =>
            paste.secret &&
            (paste.secret.session !== id ||
              paste.secret.run !== s.status?.runId),
        )
      )
        throw new Error(
          "Secret belongs to an earlier run. Remove the chip and insert it again.",
        );
      const text = composerPrompt(sent, c.pastes);
      const motion = sendMotion.prepare(text, latestSeq.current);
      try {
        setSending(true);
        try {
          const receipt = await c.sessions.send({ ...control(s), text });
          if (receipt.status === "rejected")
            throw new Error(t("Provider rejected the input"));
          if (receipt.status === "queued") {
            setQueued(text);
            clearCommand();
            if (motion) sendMotion.cancel(motion);
            return;
          }
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
      ref={conversationArea}
      className="conversation"
      aria-description={`${s?.status?.state ?? ""} · ${translateKnown(status)}`}
    >
      <SessionSearch
        source={eventListSource(transcript.events)}
        onMatchesChange={setSearchMatches}
        area={conversationArea}
        jump={(seq, inset, matchVisible) => {
          // Results belong only to the loaded window. An evicted message must
          // not turn local search into a history request.
          if (eventsRef.current.some((event) => event.seq.toString() === seq))
            return showPrompt(seq, inset, matchVisible);
        }}
        restoreFocus={() =>
          composerInput.current
            ?.querySelector<HTMLTextAreaElement>("textarea")
            ?.focus({ preventScroll: true })
        }
      />
      <Transcript
        searchMatches={searchMatches}
        loading={initialLoad && <ConversationLoading {...initialLoad} />}
        viewportCache={feed.viewport}
        pane={pane}
        source={eventListSource(transcript.events)}
        workHistory={{
          hasOlder: firstSeq !== undefined && firstSeq > floor.current + 1n,
          loadOlder: () => loadHistory("older"),
        }}
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
        jumpInset={jumpInset.current}
        onPromptJump={(seq) => void showPrompt(seq)}
        onJumped={() => setJumpTarget(undefined)}
        onNavigate={() => {
          followRef.current = false;
          setFollow(false);
        }}
        render={(e) => (
          <EventView
            source={eventSource(e)}
            agent={s?.agent ?? ""}
            completion={completions.get(e.seq.toString())}
            summary={summaries.get(e.seq.toString())}
            activitySource={snapshotSource(transcript.activities.get(e.seq))}
            loadDetails={loadDetails}
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
        {visibleApprovals.map((e) => (
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
        <p
          className={`error ${errorStyles.error} ${errorStyles.notice}`}
          role="alert"
        >
          {error || String(current.error)}
        </p>
      )}
      {needsLogin && (
        <div
          className={`${errorStyles.error} ${errorStyles.notice}`}
          role="status"
        >
          {t(
            "Claude account authentication expired. Log in again to resume this session.",
          )}
          <Button
            type="button"
            disabled={
              busy ||
              !["idle", "stopped", "failed", "interrupted"].includes(
                s?.status?.state ?? "",
              )
            }
            onClick={() =>
              void action((session) =>
                openSessionLogin(session.status?.runId ?? ""),
              )
            }
          >
            {t("Log in again")}
          </Button>
        </div>
      )}
      <ConversationComposer
        autoFocus
        initialSelection={c.drafts.selection(id)}
        onSelectionChange={(selection) => c.drafts.setSelection(id, selection)}
        mentions={
          s
            ? {
                current: s,
                sessions: workspaceContext?.sessions ?? [],
                redact: (start, end) => openSecret(false, { start, end }),
              }
            : undefined
        }
        formRef={composer}
        inputRef={composerInput}
        draft={draft}
        onDraftAssetsChange={() => c.drafts.persist(id)}
        pastes={c.pastes}
        onChange={setDraft}
        onSubmit={send}
        commands={sessionCommands(s?.agent ?? "", catalog)}
        lookupPaths={projectId ? lookupPaths : undefined}
        workspace={sessionProject?.status?.remoteWorkspace}
        canSend={!busy && !!s && !!draft.trim()}
        queued={
          <QueuedMessage
            text={queued}
            busy={busy}
            canRecall={!draft.trim()}
            recall={() =>
              void action(async (session) => {
                const text = queued;
                await c.sessions.send({ ...control(session), cancel: true });
                setQueued("");
                setDraft(text);
                c.drafts.set(id, text);
              })
            }
            cancel={() =>
              void action(async (session) => {
                await c.sessions.send({ ...control(session), cancel: true });
                setQueued("");
              })
            }
          />
        }
        sending={sending}
        busy={busy}
        turn={turn}
        backgroundJobs={background.count}
        onBackgroundJobs={() => localOpen("background")}
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
        upload={
          s?.status?.runId
            ? (file, signal, progress) =>
                uploadFile(c, id, s.status!.runId, file, signal, progress)
            : undefined
        }
        menu={(pick) => (
          <SessionMenu
            session={s && { ...s, project: sessionProject ?? s.project }}
            terminal={{
              visible: terminalVisible,
              available: !!s?.project?.id.length,
              toggle: () => showTerminal(!terminalVisibleRef.current),
            }}
            info={info}
            busy={busy}
            fork={() => forkConversation()}
            tools={[
              {
                label: "Insert secret",
                disabled: !s?.status?.runId || !projectId || busy,
                run: () => openSecret(),
              },
              { label: "Shared memory", run: () => localOpen("memory") },
              { label: "Auxiliary AI", run: () => localOpen("aux") },
              { label: "Conversation usage", run: () => localOpen("usage") },
              { label: "Background tasks", run: () => localOpen("background") },
              ...(s?.agent === "claude"
                ? [
                    {
                      label: "Log in",
                      disabled:
                        busy ||
                        !["idle", "stopped", "failed", "interrupted"].includes(
                          s.status?.state ?? "",
                        ),
                      run: () =>
                        void action((session) =>
                          openSessionLogin(session.status?.runId ?? ""),
                        ),
                    },
                  ]
                : []),
            ]}
            upload={pick}
            edit={async (field, value) => {
              const updated = await editSession(c.sessions, id, field, value);
              c.store.apply("cxz.Session", [
                { id: updated.id, value: updated },
              ]);
              return updated;
            }}
            manage={(operation, run) =>
              action(async (session) => {
                try {
                  const updated = await manageSession(
                    c.sessions,
                    create(SessionRefSchema, ref(id)),
                    operation,
                    run,
                  );
                  c.store.apply("cxz.Session", [
                    { id: updated.id, value: updated },
                  ]);
                } catch (error) {
                  if (
                    operation === "stop" ||
                    session.agent !== "claude" ||
                    !(error instanceof ConnectError) ||
                    error.code !== Code.FailedPrecondition
                  )
                    throw error;
                  const bound = await sessionLoginSnapshot(
                    c.sessions,
                    create(SessionRefSchema, ref(id)),
                    run,
                  );
                  if (
                    !requiredSessionLogin(
                      error,
                      bound.account?.alias ?? "",
                      bound.clientId,
                    )
                  )
                    throw error;
                  await openSessionLogin(run);
                }
              })
            }
          />
        )}
      >
        <ModelSettings
          session={s}
          info={info}
          catalog={catalog}
          busy={busy}
          change={(kind, value) => void changeSetting(kind, value)}
        />
        <UsageInfo info={info} agent={s?.agent ?? ""} />
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
