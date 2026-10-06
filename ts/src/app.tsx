import React, { useEffect, useMemo, useRef, useState } from "react";
import { Provider, useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "../gen/cxz/project_svc_pb";
import { SessionService } from "../gen/cxz/session_svc_pb";
import type { Session, SessionEvent } from "../gen/cxz/session_pb";
import { Connection, authenticate, ref } from "./connection";
import {
  mergeEvents,
  payload,
  detail,
  questions,
  approvalTitle,
  MAX_EVENTS,
  pendingAfter,
} from "./journal";
import { marked } from "marked";
import DOMPurify from "dompurify";
import { mergeMetadata } from "./session-metadata";
import { sessionInfo } from "./session-info";
import { ModelSettings, modelCatalog } from "./model-settings";
import { UsageInfo } from "./usage-info";
import { Button } from "./button";
import { AgentBrand } from "./agent-brand";
import { responseInfo } from "./response-info";
import {
  responseCompletions,
  type ResponseCompletion,
} from "./response-completion";
import { ResponseFooter } from "./response-footer";
import { InputMessage } from "./input-message";
import { ComposerEditor } from "./composer-editor";
import {
  BottomSheetProvider,
  BottomSheetHost,
  useBottomSheet,
} from "./bottom-sheet";
import { expandPastes } from "./composer-pastes";
import { SessionTreeGroup } from "./session-tree";
import { Transcript } from "./transcript";
export { Button } from "./button";
import "./style.css";

function ResourceIcon({ kind }: { kind: "sessions" | "projects" }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {kind === "sessions" ? (
        <path d="M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z" />
      ) : (
        <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
      )}
    </svg>
  );
}

function Markdown({ text }: { text: string }) {
  return (
    <div
      className="markdown"
      dangerouslySetInnerHTML={{
        __html: DOMPurify.sanitize(marked.parse(text, { async: false }), {
          FORBID_TAGS: ["img", "style", "input", "form"],
        }),
      }}
    />
  );
}
export function App() {
  const [connection, setConnection] = useState<Connection>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    fetch("/auth/status")
      .then((r) => {
        if (r.ok) setConnection(new Connection());
      })
      .catch(() => setError("Cannot reach cxz"));
  }, []);
  async function login(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await authenticate(token);
      setToken("");
      setConnection(new Connection());
      setError("");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function logout() {
    const r = await fetch("/auth/logout", { method: "POST" });
    if (!r.ok) throw Error("Sign out failed");
    setConnection(undefined);
  }
  if (!connection)
    return (
      <main className="login">
        <h1>cxz</h1>
        <p>Your projects, wherever you are.</p>
        <form onSubmit={login}>
          <label>
            Web access token
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoComplete="current-password"
              required
            />
          </label>
          <Button disabled={busy}>Connect</Button>
        </form>
        <p role="alert">{error}</p>
        <small>{location.origin}</small>
      </main>
    );
  return (
    <Provider app={connection}>
      <Workspace connection={connection} logout={logout} />
    </Provider>
  );
}
export function Workspace({
  connection: c,
  logout,
  initialSession = "",
  exitLabel = "Sign out",
}: {
  connection: Connection;
  logout: () => Promise<void>;
  initialSession?: string;
  exitLabel?: string;
}) {
  const [resource, setResource] = useState<"sessions" | "projects">("sessions");
  const [session, setSession] = useState(initialSession);
  const [after, setAfter] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const projects = useQuery(ProjectService.method.list, {
    filters: [{ listed: true }],
    size: 50,
    after,
  });
  const [error, setError] = useState("");
  return (
    <div
      className={`workspace ${session && resource === "sessions" ? "conversation-open" : ""}`}
    >
      <nav className="resource-sidebar" aria-label="Resources">
        <span className="brand" aria-label="cxz">
          cxz
        </span>
        {(["sessions", "projects"] as const).map((view) => (
          <Button
            key={view}
            className={`resource-link ${resource === view ? "active" : ""}`}
            aria-label={`${view === "sessions" ? "Sessions" : "Projects"} view`}
            aria-current={resource === view ? "page" : undefined}
            title={view === "sessions" ? "Sessions" : "Projects"}
            onClick={() => setResource(view)}
          >
            <ResourceIcon kind={view} />
            <span>{view === "sessions" ? "Sessions" : "Projects"}</span>
          </Button>
        ))}
      </nav>
      <aside
        className="resource-panel"
        aria-label={resource === "sessions" ? "Session list" : "Project list"}
      >
        <header>
          <strong>{resource === "sessions" ? "Sessions" : "Projects"}</strong>
          <Button onClick={() => logout().catch((e) => setError(String(e)))}>
            {exitLabel}
          </Button>
        </header>
        <p className="muted">{new URL(c.baseUrl).host}</p>
        {resource === "sessions" ? (
          <div className="session-tree" aria-label="Projects and sessions">
            {projects.data?.items.map((p) => (
              <SessionTreeGroup
                key={p.runtimeId}
                project={p}
                selected={session}
                open={!collapsed.has(p.runtimeId)}
                toggle={() =>
                  setCollapsed((old) => {
                    const next = new Set(old);
                    if (next.has(p.runtimeId)) next.delete(p.runtimeId);
                    else next.add(p.runtimeId);
                    return next;
                  })
                }
                select={setSession}
              />
            ))}
          </div>
        ) : (
          projects.data?.items.map((p) => (
            <Button
              key={p.runtimeId}
              onClick={() => {
                setCollapsed((old) => {
                  const next = new Set(old);
                  next.delete(p.runtimeId);
                  return next;
                });
                setResource("sessions");
                setSession("");
              }}
            >
              {p.name || p.alias}
            </Button>
          ))
        )}
        {after && <Button onClick={() => setAfter("")}>First projects</Button>}
        {projects.data?.next && (
          <Button onClick={() => setAfter(projects.data!.next)}>
            More projects →
          </Button>
        )}
        {(projects.error || error) && (
          <p role="alert">{String(projects.error || error)}</p>
        )}
        {projects.state === "pending" && !projects.data && (
          <p className="muted">Loading projects…</p>
        )}
      </aside>
      {resource === "projects" ? (
        <main className="resource-view">
          <header>
            <div>
              <strong>Projects</strong>
              <small>Select a project to browse its sessions.</small>
            </div>
          </header>
          <div className="resource-view-content">
            {projects.data?.items.map((p) => (
              <Button
                className="project-row"
                key={p.runtimeId}
                onClick={() => {
                  setCollapsed((old) => {
                    const next = new Set(old);
                    next.delete(p.runtimeId);
                    return next;
                  });
                  setSession("");
                  setResource("sessions");
                }}
              >
                <ResourceIcon kind="projects" />
                <span>
                  <strong>{p.name || p.alias}</strong>
                  <small>
                    {p.workspace || p.alias} · {p.status?.state}
                  </small>
                </span>
                <span aria-hidden="true">→</span>
              </Button>
            ))}
            {projects.state === "pending" && !projects.data && (
              <p className="muted">Loading projects…</p>
            )}
            {projects.data && !projects.data.items.length && (
              <p className="muted">No projects yet.</p>
            )}
          </div>
        </main>
      ) : session ? (
        <Conversation
          key={session}
          c={c}
          id={session}
          back={() => setSession("")}
        />
      ) : (
        <main className="empty">
          <h1>Your workspace</h1>
          <p>Select a session to continue.</p>
        </main>
      )}
    </div>
  );
}
function Conversation(props: { c: Connection; id: string; back: () => void }) {
  return (
    <BottomSheetProvider>
      <ConversationContent {...props} />
    </BottomSheetProvider>
  );
}
function ConversationContent({
  c,
  id,
  back,
}: {
  c: Connection;
  id: string;
  back: () => void;
}) {
  const current = useQuery(SessionService.method.get, {
    ref: ref(id),
    select: { all: true, project: { all: true } },
  });
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [metadata, setMetadata] = useState<SessionEvent[]>([]);
  const catalogRun = useRef("");
  const tension = useRef(0);
  const scrollMotion = useRef(0);
  const composerInput = useRef<HTMLDivElement>(null);
  const [pending, setPending] = useState<SessionEvent[]>([]);
  const [gap, setGap] = useState(false);
  const [status, setStatus] = useState("Connecting…");
  const [error, setError] = useState("");
  const [draft, setDraft] = useState(c.drafts.get(id) ?? "");
  const [busy, setBusy] = useState(false);
  const [follow, setFollow] = useState(true);
  const [latestShown, setLatestShown] = useState(false);
  const latestTravel = useRef({ shown: false, distance: 0 });
  useEffect(() => {
    if (!follow) return;
    latestTravel.current = { shown: false, distance: 0 };
    setLatestShown(false);
  }, [follow]);
  const pane = useRef<HTMLDivElement>(null);
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
      let before = firstSeq;
      while (before > 0n && !controller.signal.aborted) {
        const afterSeq = before > 129n ? before - 129n : 0n;
        const page = await c.sessions.history(
          { ref: ref(id), afterSeq },
          { signal: controller.signal },
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
          afterSeq === 0n ||
          !page.events.length ||
          page.events[0].seq > afterSeq + 1n
        )
          break;
        before = afterSeq + 1n;
      }
      if (!controller.signal.aborted) remember();
    })().catch((e) => {
      if (!controller.signal.aborted)
        setError(`Cannot load preceding input: ${String(e)}`);
    });
    return () => controller.abort();
  }, [c, id, firstSeq]);

  async function showPrompt(seq: string) {
    historyRequest.current?.abort();
    const controller = new AbortController();
    historyRequest.current = controller;
    pane.current?.dispatchEvent(new Event("scroll-jump"));
    followRef.current = false;
    setFollow(false);
    try {
      const target = BigInt(seq);
      const page = await c.sessions.history(
        { ref: ref(id), afterSeq: target - 1n },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      if (
        !page.events.some(
          (event) => event.seq === target && event.kind === "input",
        )
      ) {
        setError("This input is no longer available in retained history.");
        return;
      }
      detached.current = page.events.at(-1)!.seq < latestSeq.current;
      rememberMetadata(page.events);
      setEvents(page.events);
      setJumpTarget(seq);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(`Cannot open input: ${String(e)}`);
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
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
        if (cursor === 0n && latestSeq.current > 256n)
          cursor = latestSeq.current - 256n;
        const history = await c.sessions.history(
          { ref: ref(id), afterSeq: cursor },
          { signal },
        );
        if (canceled) return;
        rememberMetadata(history.events);
        if (history.events.length) {
          if (cursor > 0n && history.events[0].seq > cursor + 1n) setGap(true);
          cursor = history.events.at(-1)!.seq;
          setPending(
            pendingAfter(
              snapshot.status?.pending ?? [],
              history.events,
              snapshot.status?.lastSeq ?? 0n,
            ),
          );
          setEvents((old) =>
            detached.current ? old : mergeEvents(old, history.events),
          );
        }
        setStatus("Live");
        for await (const e of c.sessions.events(
          { ref: ref(id), afterSeq: cursor, clientId: c.clientId },
          { signal },
        )) {
          if (canceled) return;
          rememberMetadata([e]);
          cursor = e.seq > cursor ? e.seq : cursor;
          if (cursor > latestSeq.current) latestSeq.current = cursor;
          if (!followRef.current && eventsRef.current.length >= MAX_EVENTS)
            detached.current = true;
          setEvents((old) => (detached.current ? old : mergeEvents(old, [e])));
          setPending((old) => pendingAfter(old, [e]));
        }
      } catch (e) {
        if (canceled) return;
        setStatus(`Disconnected · retrying: ${String(e)}`);
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
  }, [c, id]);
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
      (direction === "older"
        ? first <= floor.current + 1n
        : last >= latestSeq.current)
    )
      return;
    if (goLatest && last >= latestSeq.current) {
      detached.current = false;
      setFollow(true);
      if (pane.current) pane.current.scrollTop = pane.current.scrollHeight;
      return;
    }
    const controller = new AbortController();
    historyRequest.current = controller;
    try {
      let cursor = goLatest
        ? latestSeq.current > 256n
          ? latestSeq.current - 256n
          : 0n
        : direction === "older"
          ? first > 129n
            ? first - 129n
            : 0n
          : last;
      let incoming: SessionEvent[] = [];
      do {
        const page = await c.sessions.history(
          { ref: ref(id), afterSeq: cursor },
          { signal: controller.signal },
        );
        const rows =
          direction === "older" && !goLatest
            ? page.events.filter((e) => e.seq < first)
            : page.events;
        incoming = mergeEvents(incoming, rows);
        const next = page.events.at(-1)?.seq ?? cursor;
        if (!goLatest || next <= cursor || next >= latestSeq.current) break;
        cursor = next;
      } while (!controller.signal.aborted);
      if (controller.signal.aborted) return;
      if (!incoming.length) {
        if (direction === "older") {
          floor.current = first - 1n;
          updateShadow();
        }
        return;
      }
      const next = goLatest
        ? incoming
        : mergeEvents(eventsRef.current, incoming, direction);
      detached.current = next.at(-1)!.seq < latestSeq.current;
      setEvents(next);
      if (goLatest) setFollow(true);
    } catch (e) {
      if (!controller.signal.aborted)
        setError(`Cannot load history: ${String(e)}`);
    } finally {
      if (historyRequest.current === controller) historyRequest.current = null;
    }
  }
  const visibleEvents = useMemo(
    () =>
      events.filter(
        (e) =>
          !["state", "approval_resolved", "models", "usage"].includes(e.kind),
      ),
    [events],
  );
  const s = current.data;
  const completions = useMemo(() => responseCompletions(events), [events]);
  const combined = useMemo(
    () =>
      [
        ...new Map([...events, ...metadata].map((e) => [e.seq, e])).values(),
      ].sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0)),
    [events, metadata],
  );
  const info = useMemo(() => sessionInfo(s, combined), [s, combined]);
  const catalog = useMemo(() => modelCatalog(s, combined), [s, combined]);
  // A long transcript's model catalog may precede its retained event window.
  // Read history like the TUI; never send an unsupported slash command to chat.
  useEffect(() => {
    const run = s?.status?.runId;
    if (!run || !events.length || catalog || catalogRun.current === run) return;
    catalogRun.current = run;
    const controller = new AbortController();
    void (async () => {
      let afterSeq = 0n;
      while (!controller.signal.aborted) {
        const page = await c.sessions.history(
          { ref: ref(id), afterSeq },
          { signal: controller.signal },
        );
        rememberMetadata(page.events);
        const next = page.events.at(-1)?.seq ?? afterSeq;
        if (next <= afterSeq || next >= latestSeq.current) break;
        afterSeq = next;
      }
    })().catch((e) => {
      if (!controller.signal.aborted)
        setError(`Cannot read model choices: ${String(e)}`);
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
    if (!catalog || s?.status?.state !== "idle") return;
    await action(async (session) => {
      async function apply(name: "model" | "effort", choice: string) {
        const command = control(session);
        let afterSeq = latestSeq.current;
        const receipt = await c.sessions.send({
          ...command,
          text: `/${name} ${choice}`,
        });
        if (receipt.status === "accepted") return;
        if (receipt.status === "rejected")
          throw new Error("Provider rejected the setting");
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
                throw new Error("Provider rejected the setting");
            }
            afterSeq = page.events.at(-1)?.seq ?? afterSeq;
            await new Promise((resolve) => setTimeout(resolve, 100));
          }
          throw new Error("Provider has not confirmed the setting");
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
    if (lock.current || !s) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      await fn(s);
    } catch (e) {
      setError(
        `${String(e)}. The result may be unknown after a disconnect; inspect the conversation before retrying.`,
      );
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
    await action(async (s) => {
      const receipt = await c.sessions.send({
        ...control(s),
        text: expandPastes(sent, c.pastes),
      });
      if (receipt.status === "rejected")
        throw new Error("Provider rejected the input");
      setDraft((old) => (old === sent ? "" : old));
      if (detached.current) await loadHistory("newer", true);
      pane.current?.dispatchEvent(new Event("scroll-jump"));
      setFollow(true);
      if (pane.current) pane.current.scrollTop = pane.current.scrollHeight;
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
    <main className="conversation">
      <header>
        <Button onClick={back} aria-label="Back to sessions">
          ←
        </Button>
        <div>
          <strong>{s?.alias || s?.runtimeId || "Session"}</strong>
          <small>
            <AgentBrand agent={s?.agent ?? ""} /> · {s?.status?.state} ·{" "}
            {status}
          </small>
        </div>
      </header>
      <Transcript
        pane={pane}
        events={visibleEvents}
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
          />
        )}
        notice={
          gap && (
            <p className="muted history-note">
              Earlier history is no longer available.
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
          const next =
            !reading &&
            !detached.current &&
            el.scrollHeight - el.scrollTop - el.clientHeight < 1;
          followRef.current = next;
          setFollow(next);
        }}
      />
      <section className="pending">
        {pending.map((e) => (
          <Approval
            key={`${e.runId}:${e.requestId}`}
            e={e}
            agent={s?.agent ?? ""}
            busy={busy}
            reply={reply}
          />
        ))}
      </section>
      {!!(error || current.error) && (
        <p className="error" role="alert">
          {error || String(current.error)}
        </p>
      )}
      <form className="composer" onSubmit={send}>
        <div className="composer-wrapper">
          <BottomSheetHost />
          <div className="composer-toolbar">
            <span
              className="latest-slot"
              data-visible={!follow && latestShown}
              inert={follow || !latestShown}
              aria-hidden={follow || !latestShown}
            >
              <Button
                className="toolbar-button latest-button"
                type="button"
                aria-label="Latest"
                onClick={() => void loadHistory("newer", true)}
              >
                <svg
                  width="18"
                  height="18"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M12 5v14m-6-6 6 6 6-6" />
                </svg>
              </Button>
            </span>
            <span className="send-control">
              <Button
                className="toolbar-button send"
                type="submit"
                aria-label="Send"
                aria-keyshortcuts="Control+Enter"
                aria-describedby="send-shortcut"
                disabled={busy || !s || !draft.trim()}
              >
                <svg
                  width="18"
                  height="18"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M12 19V5m-6 6 6-6 6 6" />
                </svg>
              </Button>
              <span id="send-shortcut" className="send-shortcut" role="tooltip">
                ctrl+enter
              </span>
            </span>
          </div>
          <div className="composer-input" ref={composerInput}>
            <ComposerEditor
              value={draft}
              onChange={setDraft}
              pastes={c.pastes}
              canSend={!busy && !!s && !!draft.trim()}
            />
          </div>
        </div>
        <div className="composer-meta" aria-label="Session information">
          <ModelSettings
            session={s}
            info={info}
            catalog={catalog}
            busy={busy}
            change={(kind, value) => void changeSetting(kind, value)}
          />
          <UsageInfo info={info} />
        </div>
      </form>
    </main>
  );
}
const EventView = React.memo(
  function EventView({
    e,
    agent,
    completion,
  }: {
    e: SessionEvent;
    agent: string;
    completion?: ResponseCompletion;
  }) {
    const openSheet = useBottomSheet();
    if (e.kind === "assistant") {
      const info = responseInfo(e.response);
      return (
        <article className="response" data-seq={e.seq.toString()}>
          <small className="response-heading">
            <AgentBrand agent={agent} />
            {info.label && (
              <span className="response-settings" title={info.description}>
                {info.label}
              </span>
            )}
          </small>
          <Markdown text={e.text} />
          <ResponseFooter
            seq={e.seq}
            timeMs={e.timeMs}
            text={e.text}
            completion={completion}
          />
        </article>
      );
    }
    if (e.kind === "input")
      return (
        <article className="input" data-seq={e.seq.toString()}>
          <InputMessage event={e} />
        </article>
      );
    return (
      <Button
        type="button"
        data-seq={e.seq.toString()}
        className={`event-detail ${e.kind === "diagnostic" || e.kind === "stderr" ? "error" : ""}`}
        onClick={() =>
          openSheet({
            title: e.kind === "approval" ? approvalTitle(e) : e.kind,
            content: () => (
              <>
                <pre>{e.text}</pre>
                {e.payload.length > 0 && <pre>{detail(e)}</pre>}
              </>
            ),
          })
        }
      >
        {e.kind === "approval"
          ? approvalTitle(e)
          : `${e.kind} · ${e.text.slice(0, 160)}`}
      </Button>
    );
  },
  (previous, next) =>
    previous.e === next.e &&
    previous.agent === next.agent &&
    JSON.stringify(previous.completion) === JSON.stringify(next.completion),
);
function Approval({
  e,
  agent,
  busy,
  reply,
}: {
  e: SessionEvent;
  agent: string;
  busy: boolean;
  reply: (e: SessionEvent, allow: boolean, answers?: string) => Promise<void>;
}) {
  const openSheet = useBottomSheet();
  const qs = questions(agent, e);
  const [selected, setSelected] = useState<Record<string, string[]>>({});
  const [other, setOther] = useState<Record<string, string>>({});
  const elicitation = e.text === "mcpServer/elicitation/request";
  const p = payload(e);
  const requiresForm =
    (elicitation && p.params?.requestedSchema) ||
    e.text === "agentMessage/questions";
  return (
    <section className="approval">
      <h3>{approvalTitle(e)}</h3>
      {p.params?.message && <p>{String(p.params.message)}</p>}
      <Button
        type="button"
        className="event-detail"
        onClick={() =>
          openSheet({
            title: "Request details",
            content: () => <pre>{detail(e)}</pre>,
          })
        }
      >
        Request details
      </Button>
      {qs.map((q) => (
        <fieldset key={q.key}>
          <legend>{q.text}</legend>
          {q.options.map((o) => (
            <label key={o.label}>
              <input
                type={q.multi ? "checkbox" : "radio"}
                name={`${e.requestId}:${q.key}`}
                checked={(selected[q.key] ?? []).includes(o.label)}
                onChange={(event) =>
                  setSelected((old) => ({
                    ...old,
                    [q.key]: q.multi
                      ? event.target.checked
                        ? [...(old[q.key] ?? []), o.label]
                        : (old[q.key] ?? []).filter((x) => x !== o.label)
                      : [o.label],
                  }))
                }
              />
              {o.label}
              {o.description && <small>{o.description}</small>}
            </label>
          ))}
          {q.other && (
            <input
              aria-label={`Other answer: ${q.text}`}
              type={q.secret ? "password" : "text"}
              value={other[q.key] ?? ""}
              onChange={(event) =>
                setOther((old) => ({ ...old, [q.key]: event.target.value }))
              }
            />
          )}
        </fieldset>
      ))}
      {requiresForm && (
        <p>
          This request form is not supported in the web client yet. Complete it
          in the TUI.
        </p>
      )}
      <div className="buttons">
        <Button
          disabled={
            busy ||
            !!requiresForm ||
            qs.some((q) => !(selected[q.key]?.length || other[q.key]?.trim()))
          }
          onClick={() =>
            reply(
              e,
              true,
              qs.length
                ? JSON.stringify(
                    Object.fromEntries(
                      qs.map((q) => [
                        q.key,
                        {
                          selected: selected[q.key] ?? [],
                          other: other[q.key] ?? "",
                        },
                      ]),
                    ),
                  )
                : "",
            )
          }
        >
          {qs.length ? "Submit answers" : "Allow"}
        </Button>
        <Button disabled={busy} onClick={() => reply(e, false)}>
          Deny
        </Button>
      </div>
    </section>
  );
}
