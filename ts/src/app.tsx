import { t, translateKnown } from "./i18n";
import { useLocale } from "./i18n-react";
import React, {
  createContext,
  useContext,
  type PropsWithChildren,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Provider, useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "../gen/cxz/project_svc_pb";
import { SessionService } from "../gen/cxz/session_svc_pb";
import type { Project } from "../gen/cxz/project_pb";
import type { Session, SessionEvent } from "../gen/cxz/session_pb";
import { Connection, authenticate, ref } from "./connection";
import {
  SessionHistory,
  type HistoryPage,
  type LoadEventDetails,
} from "./session-history";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "../gen/cxz/session_pb";
import { EventDetails } from "./event-details";
import {
  mergeEvents,
  isTranscriptEvent,
  payload,
  detail,
  questions,
  approvalTitle,
  pendingAfter,
} from "./journal";
import { marked } from "marked";
import DOMPurify from "dompurify";
import { mergeMetadata } from "./session-metadata";
import { sessionInfo } from "./session-info";
import { ModelSettings, modelCatalog } from "./model-settings";
import { UsageInfo } from "./usage-info";
import { Button } from "./button";
import { TurnControls } from "./turn-controls";
import { advanceTurn, snapshotTurn, type TurnProgress } from "./turn-progress";
import { AgentBrand } from "./agent-brand";
import { responseInfo } from "./response-info";
import type { ResponseCompletion } from "./response-completion";
import { ResponseFooter } from "./response-footer";
import { InputMessage } from "./input-message";
import { ComposerEditor } from "./composer-editor";
import { sessionCommands } from "./composer-commands";
import { useSendMotion } from "./send-motion";
import { WorkspaceTerminal, terminalShortcut } from "./workspace-terminal";
import {
  FloatingCardProvider,
  FloatingCardHost,
  FloatingCard,
  useFloatingCard,
} from "./floating-card";
import { type ComposerPaste } from "./composer-pastes";
import { composerPrompt } from "./composer-code";
import { SessionTreeGroup } from "./session-tree";
import { SessionIdentity } from "./session-identity";
import { ActionMenu } from "./action-menu";
import { PanelScroll } from "./panel-scroll";
import { useScrollbars } from "./scrollbars";
import { useResourceInventory } from "./resource-inventory";
import { key } from "@lesomnus/payday/store";
import { transcriptEvents, type ToolActivity } from "./tool-activity";
import { ToolActivityView } from "./tool-activity-view";
import { Transcript } from "./transcript";
import { WorkspaceEditor } from "./workspace-editor";
import { SettingsPage } from "./settings-page";
import { useNavigate } from "@tanstack/react-router";
import { RouteLink } from "./route-link";
import { useWorkspaceRoute } from "./router";
export { Button } from "./button";
import "./style.css";

function ResourceIcon({
  kind,
}: {
  kind: "sessions" | "projects" | "settings";
}) {
  useLocale();
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
      ) : kind === "projects" ? (
        <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
      ) : (
        <>
          <circle cx="12" cy="12" r="3" />
          <path d="M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1Z" />
        </>
      )}
    </svg>
  );
}

function Markdown({ text }: { text: string }) {
  useLocale();
  return (
    <div
      className="markdown"
      dangerouslySetInnerHTML={{
        __html: DOMPurify.sanitize(marked.parse(text, { async: false }), {
          FORBID_TAGS: ["img", "style", "input", "form"],
          FORBID_ATTR: ["style"],
        }),
      }}
    />
  );
}
export function App({ children }: PropsWithChildren) {
  useLocale();
  const [connection, setConnection] = useState<Connection>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    fetch("/auth/status")
      .then((r) => {
        if (r.ok) setConnection(new Connection());
      })
      .catch(() => setError(t("Cannot reach cxz")));
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
    if (!r.ok) throw Error(t("Sign out failed"));
    setConnection(undefined);
  }
  if (!connection)
    return (
      <main className="login">
        <h1>cxz</h1>
        <p>{t("Your projects, wherever you are.")}</p>
        <form onSubmit={login}>
          <label>
            {t("Web access token")}
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoComplete="current-password"
              required
            />
          </label>
          <Button disabled={busy}>{t("Connect")}</Button>
        </form>
        <p role="alert">{error}</p>
        <small>{location.origin}</small>
      </main>
    );
  return (
    <Provider app={connection}>
      <Workspace connection={connection} logout={logout}>
        {children}
      </Workspace>
    </Provider>
  );
}
export function Workspace({
  connection: c,
  logout,
  exitLabel = t("Sign out"),
  children,
}: {
  connection: Connection;
  logout: () => Promise<void>;
  exitLabel?: string;
} & PropsWithChildren) {
  useLocale();
  useScrollbars();
  const route = useWorkspaceRoute();
  const resource = route.resource === "not-found" ? "sessions" : route.resource;
  const session = route.session;
  const settingsTopic = route.settingsTopic;
  const lastSession = useRef("");
  if (route.resource === "sessions") lastSession.current = session;
  const [settingsFileOpen, setSettingsFileOpen] = useState(false);
  useEffect(() => setSettingsFileOpen(false), [resource, settingsTopic]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const inventory = useResourceInventory(c);
  const projects = inventory.projects;
  const sessionsByProject = useMemo(() => {
    const grouped = new Map<string, Session[]>();
    for (const session of inventory.sessions) {
      if (!session.project) continue;
      const id = key(session.project.id);
      const sessions = grouped.get(id) ?? [];
      sessions.push(session);
      grouped.set(id, sessions);
    }
    return grouped;
  }, [inventory.sessions]);
  const [error, setError] = useState("");
  function expandProject(id: string) {
    setCollapsed((old) => {
      const next = new Set(old);
      next.delete(id);
      return next;
    });
  }
  return (
    <WorkspaceContext.Provider
      value={{
        c,
        projects,
        projectsLoading: inventory.loading.projects,
        settingsFileOpen,
        setSettingsFileOpen,
        expandProject,
      }}
    >
      <div
        className={`workspace ${(session && resource === "sessions") || route.resource === "not-found" ? "conversation-open" : ""} ${resource === "settings" ? "settings-open" : ""}`}
      >
        <nav className="resource-sidebar" aria-label={t("Resources")}>
          <span className="brand" aria-label="cxz">
            cxz
          </span>
          {(["sessions", "projects"] as const).map((view) => (
            <RouteLink
              key={view}
              {...(view === "projects"
                ? { to: "/projects" as const }
                : lastSession.current
                  ? {
                      to: "/sessions/$sessionId" as const,
                      params: { sessionId: lastSession.current },
                    }
                  : { to: "/sessions" as const })}
              className={`resource-link ${resource === view ? "active" : ""}`}
              aria-label={
                view === "sessions" ? t("Sessions view") : t("Projects view")
              }
              aria-current={resource === view ? "page" : undefined}
              title={view === "sessions" ? t("Sessions") : t("Projects")}
            >
              <ResourceIcon kind={view} />
              <span>{view === "sessions" ? t("Sessions") : t("Projects")}</span>
            </RouteLink>
          ))}
          <RouteLink
            to="/settings/general"
            className={`resource-link settings-link ${resource === "settings" ? "active" : ""}`}
            aria-label={t("Settings view")}
            aria-current={resource === "settings" ? "page" : undefined}
            title={t("Settings")}
            onClick={() => {
              setSettingsFileOpen(false);
            }}
          >
            <ResourceIcon kind="settings" />
            <span>{t("Settings")}</span>
          </RouteLink>
        </nav>
        <aside
          className={`resource-panel${resource === "sessions" ? " session-panel" : ""}`}
          aria-label={
            resource === "sessions"
              ? t("Session list")
              : resource === "settings"
                ? t("Settings navigation")
                : t("Project list")
          }
        >
          <header>
            <strong>
              {resource === "sessions"
                ? t("Sessions")
                : resource === "settings"
                  ? t("Settings")
                  : t("Projects")}
            </strong>
            <Button onClick={() => logout().catch((e) => setError(String(e)))}>
              {exitLabel}
            </Button>
          </header>
          <p className="muted">{new URL(c.baseUrl).host}</p>
          <PanelScroll>
            {resource === "settings" ? (
              <nav
                className="settings-topics"
                aria-label={t("Settings topics")}
              >
                {(["general", "editor"] as const).map((topic) => (
                  <RouteLink
                    key={topic}
                    to={
                      topic === "general"
                        ? "/settings/general"
                        : "/settings/editor"
                    }
                    aria-current={settingsTopic === topic ? "page" : undefined}
                    aria-controls="settings-editor"
                    onClick={() => {
                      setSettingsFileOpen(false);
                    }}
                  >
                    {topic === "general" ? t("General") : t("Editor")}
                  </RouteLink>
                ))}
              </nav>
            ) : resource === "sessions" ? (
              <div
                className="session-tree"
                aria-label={t("Projects and sessions")}
              >
                {projects.map((p) => (
                  <SessionTreeGroup
                    key={p.runtimeId}
                    project={p}
                    items={sessionsByProject.get(key(p.id)) ?? []}
                    loading={inventory.loading.sessions}
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
                  />
                ))}
              </div>
            ) : (
              projects.map((p) => (
                <RouteLink
                  key={p.runtimeId}
                  to="/sessions"
                  onClick={() => expandProject(p.runtimeId)}
                >
                  {p.name || p.alias}
                </RouteLink>
              ))
            )}
            {(inventory.errors.projects ||
              inventory.errors.sessions ||
              error) && (
              <p role="alert">
                {String(
                  inventory.errors.projects ||
                    inventory.errors.sessions ||
                    error,
                )}
              </p>
            )}
            {inventory.loading.projects && !projects.length && (
              <p className="muted">{t("Loading projects…")}</p>
            )}
          </PanelScroll>
        </aside>
        {children}
      </div>
    </WorkspaceContext.Provider>
  );
}

const WorkspaceContext = createContext<
  | {
      c: Connection;
      projects: Project[];
      projectsLoading: boolean;
      settingsFileOpen: boolean;
      setSettingsFileOpen: (open: boolean) => void;
      expandProject: (id: string) => void;
    }
  | undefined
>(undefined);

export function WorkspaceView() {
  useLocale();
  const {
    c,
    projects,
    projectsLoading,
    settingsFileOpen,
    setSettingsFileOpen,
    expandProject,
  } = useContext(WorkspaceContext)!;
  const { resource, session, settingsTopic } = useWorkspaceRoute();
  const navigate = useNavigate();
  if (resource === "not-found")
    return (
      <main className="empty route-not-found">
        <h1>{t("Page not found")}</h1>
        <RouteLink to="/sessions">{t("Back to sessions")}</RouteLink>
      </main>
    );
  return (
    <>
      {resource === "settings" ? (
        <SettingsPage
          topic={settingsTopic}
          fileOpen={settingsFileOpen}
          setFileOpen={setSettingsFileOpen}
        />
      ) : resource === "projects" ? (
        <main className="resource-view">
          <header>
            <div>
              <strong>{t("Projects")}</strong>
              <small>{t("Select a project to browse its sessions.")}</small>
            </div>
          </header>
          <div className="resource-view-content">
            {projects.map((p) => (
              <RouteLink
                to="/sessions"
                className="project-row"
                key={p.runtimeId}
                onClick={() => expandProject(p.runtimeId)}
              >
                <ResourceIcon kind="projects" />
                <span>
                  <strong>{p.name || p.alias}</strong>
                  <small>
                    {p.workspace || p.alias} · {p.status?.state}
                  </small>
                </span>
                <span aria-hidden="true">→</span>
              </RouteLink>
            ))}
            {projectsLoading && !projects.length && (
              <p className="muted">{t("Loading projects…")}</p>
            )}
            {!projectsLoading && !projects.length && (
              <p className="muted">{t("No projects yet.")}</p>
            )}
          </div>
        </main>
      ) : session ? (
        <SessionWorkspace
          c={c}
          id={session}
          back={() => void navigate({ to: "/sessions" })}
        />
      ) : (
        <main className="empty">
          <h1>{t("Your workspace")}</h1>
          <p>{t("Select a session to continue.")}</p>
        </main>
      )}
    </>
  );
}
function SessionWorkspace({
  c,
  id,
  back,
}: {
  c: Connection;
  id: string;
  back: () => void;
}) {
  useLocale();
  const current = useQuery(SessionService.method.get, {
    ref: ref(id),
    select: { all: true, project: { all: true } },
  });
  const area = useRef<HTMLDivElement>(null);
  const [activated, setActivated] = useState(false);
  useEffect(() => {
    const observer = new ResizeObserver(([entry]) => {
      if (entry.contentRect.width >= 1600) setActivated(true);
    });
    observer.observe(area.current!);
    return () => observer.disconnect();
  }, []);
  const project = current.data?.project;
  return (
    <div className="session-workspace" ref={area}>
      <div className="session-split">
        <Conversation key={id} c={c} id={id} back={back} />
        {activated && !!project?.id.length && (
          <ProjectEditorPane
            key={Array.from(project.id).join("-")}
            c={c}
            projectId={project.id}
          />
        )}
      </div>
    </div>
  );
}
function ProjectEditorPane({
  c,
  projectId,
}: {
  c: Connection;
  projectId: Uint8Array;
}) {
  useLocale();
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  return project.data ? (
    <WorkspaceEditor c={c} project={project.data} />
  ) : (
    <aside className="workspace-editor" aria-label={t("Workspace editor")}>
      <p role={project.error ? "alert" : "status"}>
        {project.error ? String(project.error) : t("Loading workspace…")}
      </p>
    </aside>
  );
}
function Conversation(props: { c: Connection; id: string; back: () => void }) {
  useLocale();
  return (
    <FloatingCardProvider>
      <ConversationContent {...props} />
    </FloatingCardProvider>
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
  useLocale();
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
  const [pending, setPending] = useState<SessionEvent[]>([]);
  const [gap, setGap] = useState(false);
  const [status, setStatus] = useState("Connecting…");
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
        setTurn((old) => snapshotTurn(snapshot.status, loaded, old));
        setStatus("Live");
        for await (const e of c.sessions.events(
          { ref: ref(id), afterSeq: cursor, clientId: c.clientId },
          { signal },
        )) {
          if (canceled) return;
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
      } else if (await changeSetting(kind, setting[2])) {
        clearCommand();
      }
      return;
    }
    await action(async (s) => {
      const text = composerPrompt(sent, c.pastes);
      const motion = sendMotion.prepare(text, latestSeq.current);
      try {
        const receipt = await c.sessions.send({ ...control(s), text });
        if (receipt.status === "rejected")
          throw new Error(t("Provider rejected the input"));
        // A session switch can unmount this view during the decorative departure.
        if (c.drafts.get(id) === sent) c.drafts.set(id, "");
        await sendMotion.depart(motion);
        setDraft((old) => (old === sent ? "" : old));
        if (detached.current) await loadHistory("newer", true);
        pane.current?.dispatchEvent(new Event("scroll-jump"));
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
    <main className="conversation">
      <header>
        <Button
          className="conversation-back"
          onClick={back}
          aria-label={t("Back to sessions")}
        >
          ←
        </Button>
        <SessionIdentity session={s} heading />
        <ActionMenu
          label={t("Session menu")}
          status={`${s?.status?.state ?? ""} · ${translateKnown(status)}`}
          items={[
            {
              label: t("Terminal"),
              shortcut: "Ctrl+`",
              checked: terminalVisible,
              run: () => showTerminal(!terminalVisibleRef.current),
            },
          ]}
        />
      </header>
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
          const next =
            !reading &&
            !detached.current &&
            el.scrollHeight - el.scrollTop - el.clientHeight < 1;
          followRef.current = next;
          setFollow(next);
        }}
      />
      <FloatingCardHost>
        {pending.map((e) => (
          <Approval
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
      <form className="composer" onSubmit={send}>
        <div className="composer-wrapper">
          <div className="composer-toolbar">
            <TurnControls
              turn={turn}
              busy={busy}
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
            />
            <span
              className="latest-slot"
              data-visible={!follow && latestShown}
              inert={follow || !latestShown}
              aria-hidden={follow || !latestShown}
            >
              <Button
                className="toolbar-button latest-button"
                type="button"
                aria-label={t("Latest")}
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
                aria-label={t("Send")}
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
              commands={sessionCommands(s?.agent ?? "", catalog)}
            />
          </div>
        </div>
        <div className="composer-meta" aria-label={t("Session information")}>
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
const EventView = React.memo(
  function EventView({
    e,
    agent,
    completion,
    activity,
    loadDetails,
  }: {
    e: SessionEvent;
    agent: string;
    completion?: ResponseCompletion;
    activity?: ToolActivity;
    loadDetails?: LoadEventDetails;
  }) {
    useLocale();
    const openCard = useFloatingCard();
    if (activity)
      return (
        <ToolActivityView
          activity={activity}
          agent={agent}
          seq={e.seq}
          loadDetails={loadDetails}
        />
      );
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
          openCard({
            title: e.kind === "approval" ? approvalTitle(e) : e.kind,
            content: () => (
              <EventDetails
                event={e}
                loadDetails={e.payload.length ? undefined : loadDetails}
              />
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
    previous.activity === next.activity &&
    previous.loadDetails === next.loadDetails &&
    JSON.stringify(previous.completion) === JSON.stringify(next.completion),
);
function Approval({
  e,
  agent,
  pastes,
  busy,
  reply,
}: {
  e: SessionEvent;
  agent: string;
  pastes: Map<string, ComposerPaste>;
  busy: boolean;
  reply: (e: SessionEvent, allow: boolean, answers?: string) => Promise<void>;
}) {
  useLocale();
  const openCard = useFloatingCard();
  const qs = questions(agent, e);
  const [selected, setSelected] = useState<Record<string, string[]>>({});
  const [other, setOther] = useState<Record<string, string>>({});
  const otherAnswer = (key: string) => composerPrompt(other[key] ?? "", pastes);
  const elicitation = e.text === "mcpServer/elicitation/request";
  const p = payload(e);
  const requiresForm =
    (elicitation && p.params?.requestedSchema) ||
    e.text === "agentMessage/questions";
  return (
    <FloatingCard
      className="approval"
      title={approvalTitle(e)}
      role="region"
      aria-label={approvalTitle(e)}
    >
      {p.params?.message && <p>{String(p.params.message)}</p>}
      <Button
        type="button"
        className="event-detail"
        onClick={() =>
          openCard({
            title: () => t("Request details"),
            content: () => <pre>{detail(e)}</pre>,
          })
        }
      >
        {t("Request details")}
      </Button>
      {qs.map((q) => (
        <fieldset key={q.key} className="question-group">
          <legend>{q.text}</legend>
          <div className="question-options">
            {q.options.map((o) => (
              <label
                key={o.label}
                className="question-option"
                data-selected={(selected[q.key] ?? []).includes(o.label)}
              >
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
                <span className="question-option-text">
                  <strong>{o.label}</strong>
                  {o.description && <small>{o.description}</small>}
                </span>
              </label>
            ))}
          </div>
          {q.other && (
            <div className="question-other">
              <span className="question-other-label">{t("Other")}</span>
              {q.secret ? (
                <input
                  aria-label={t("Other answer: {question}", {
                    question: q.text,
                  })}
                  type="password"
                  value={other[q.key] ?? ""}
                  onChange={(event) =>
                    setOther((old) => ({ ...old, [q.key]: event.target.value }))
                  }
                />
              ) : (
                <div className="question-other-editor">
                  <ComposerEditor
                    ariaLabel={t("Other answer: {question}", {
                      question: q.text,
                    })}
                    placeholder={t("Type your answer…")}
                    value={other[q.key] ?? ""}
                    pastes={pastes}
                    onChange={(value) =>
                      setOther((old) => ({ ...old, [q.key]: value }))
                    }
                  />
                </div>
              )}
            </div>
          )}
        </fieldset>
      ))}
      {requiresForm && (
        <p>
          {t(
            "This request form is not supported in the web client yet. Complete it in the TUI.",
          )}
        </p>
      )}
      <div className="buttons">
        <Button
          disabled={
            busy ||
            !!requiresForm ||
            qs.some(
              (q) => !(selected[q.key]?.length || otherAnswer(q.key).trim()),
            )
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
                          other: otherAnswer(q.key),
                        },
                      ]),
                    ),
                  )
                : "",
            )
          }
        >
          {qs.length ? t("Submit answers") : t("Allow")}
        </Button>
        <Button disabled={busy} onClick={() => reply(e, false)}>
          {t("Deny")}
        </Button>
      </div>
    </FloatingCard>
  );
}
