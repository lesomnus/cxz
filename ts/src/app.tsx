import React, { useEffect, useRef, useState } from "react";
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
import "./style.css";

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
          <button disabled={busy}>Connect</button>
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
  const [project, setProject] = useState("");
  const [session, setSession] = useState(initialSession);
  const [after, setAfter] = useState("");
  const [sessionAfter, setSessionAfter] = useState("");
  const projects = useQuery(ProjectService.method.list, {
    filters: [{ listed: true }],
    size: 50,
    after,
  });
  const sessions = useQuery(SessionService.method.list, {
    filters: [{ listed: true, ...(project ? { project: ref(project) } : {}) }],
    size: 50,
    after: sessionAfter,
  });
  const [error, setError] = useState("");
  return (
    <div className={`workspace ${session ? "conversation-open" : ""}`}>
      <aside>
        <header>
          <strong>cxz</strong>
          <button onClick={() => logout().catch((e) => setError(String(e)))}>
            {exitLabel}
          </button>
        </header>
        <p className="muted">{new URL(c.baseUrl).host}</p>
        <h2>Projects</h2>
        <button
          className={!project ? "active" : ""}
          onClick={() => {
            setProject("");
            setSessionAfter("");
          }}
        >
          All projects
        </button>
        {projects.data?.items.map((p) => (
          <button
            key={p.runtimeId}
            className={project === p.runtimeId ? "active" : ""}
            onClick={() => {
              setProject(p.runtimeId);
              setSessionAfter("");
            }}
          >
            {p.name || p.alias}
            <small>{p.status?.state}</small>
          </button>
        ))}
        {after && <button onClick={() => setAfter("")}>First projects</button>}
        {projects.data?.next && (
          <button onClick={() => setAfter(projects.data!.next)}>
            More projects →
          </button>
        )}
        <h2>Sessions</h2>
        {sessions.data?.items.map((s) => (
          <button
            key={s.runtimeId}
            className={session === s.runtimeId ? "active" : ""}
            onClick={() => setSession(s.runtimeId)}
          >
            {s.name || s.alias}
            <small>
              {s.agent} · {s.status?.state}
            </small>
          </button>
        ))}
        {sessionAfter && (
          <button onClick={() => setSessionAfter("")}>First sessions</button>
        )}
        {sessions.data?.next && (
          <button onClick={() => setSessionAfter(sessions.data!.next)}>
            More sessions →
          </button>
        )}
        {projects.error || sessions.error || error ? (
          <p role="alert">
            {String(projects.error || sessions.error || error)}
          </p>
        ) : null}
        {sessions.state === "pending" && !sessions.data && (
          <p>Loading sessions…</p>
        )}
      </aside>
      {session ? (
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
function Conversation({
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
  const [pending, setPending] = useState<SessionEvent[]>([]);
  const [gap, setGap] = useState(false);
  const [status, setStatus] = useState("Connecting…");
  const [error, setError] = useState("");
  const [draft, setDraft] = useState(c.drafts.get(id) ?? "");
  const [busy, setBusy] = useState(false);
  const [follow, setFollow] = useState(true);
  const pane = useRef<HTMLDivElement>(null);
  const lock = useRef(false);
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
        const history = await c.sessions.history(
          { ref: ref(id), afterSeq: cursor },
          { signal },
        );
        if (canceled) return;
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
          setEvents((old) => mergeEvents(old, history.events));
        }
        setStatus("Live");
        for await (const e of c.sessions.events(
          { ref: ref(id), afterSeq: cursor, clientId: c.clientId },
          { signal },
        )) {
          if (canceled) return;
          cursor = e.seq > cursor ? e.seq : cursor;
          setEvents((old) => mergeEvents(old, [e]));
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
  useEffect(() => {
    if (follow && pane.current)
      pane.current.scrollTop = pane.current.scrollHeight;
  }, [events, follow]);
  const s = current.data;
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
      await c.sessions.send({ ...control(s), text: sent });
      setDraft((old) => (old === sent ? "" : old));
      setFollow(true);
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
        <button onClick={back} aria-label="Back to sessions">
          ←
        </button>
        <div>
          <strong>{s?.name || s?.alias || "Session"}</strong>
          <small>
            {s?.agent} · {s?.status?.state} · {status}
          </small>
        </div>
        <details className="actions">
          <summary>Actions</summary>
          <button
            disabled={busy}
            onClick={() => action((s) => c.sessions.interrupt(control(s)))}
          >
            Interrupt turn
          </button>
          <button
            disabled={busy}
            onClick={() =>
              action((s) =>
                c.queries.call(SessionService.method.stop, control(s)),
              )
            }
          >
            Stop session
          </button>
          <button
            disabled={busy}
            onClick={() =>
              action((s) =>
                c.queries.call(SessionService.method.resume, control(s)),
              )
            }
          >
            Resume session
          </button>
        </details>
      </header>
      <div
        className="transcript"
        ref={pane}
        onScroll={() => {
          const el = pane.current!;
          setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 100);
        }}
      >
        <p className="muted">
          Retained history · up to {MAX_EVENTS} recent events in this view
          {gap && " · Some events were removed by retention while disconnected"}
        </p>
        {events
          .filter((e) => !["state", "approval_resolved"].includes(e.kind))
          .map((e) => (
            <EventView key={e.seq.toString()} e={e} agent={s?.agent ?? ""} />
          ))}
      </div>
      {!follow && (
        <button className="bottom" onClick={() => setFollow(true)}>
          ↓ Latest
        </button>
      )}
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
        <textarea
          aria-label="Message"
          placeholder="Continue the conversation…"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          rows={3}
        />
        <button disabled={busy || !s || !draft.trim()}>Send</button>
      </form>
    </main>
  );
}
function EventView({ e, agent }: { e: SessionEvent; agent: string }) {
  if (e.kind === "assistant")
    return (
      <article>
        <small>{agent.toUpperCase()}</small>
        <Markdown text={e.text} />
        <button
          className="copy"
          onClick={() => navigator.clipboard.writeText(e.text).catch(() => {})}
        >
          Copy
        </button>
      </article>
    );
  if (e.kind === "input")
    return (
      <article className="input">
        <small>❯ You</small>
        <p>{e.text}</p>
      </article>
    );
  return (
    <details
      className={e.kind === "diagnostic" || e.kind === "stderr" ? "error" : ""}
    >
      <summary>
        {e.kind === "approval"
          ? approvalTitle(e)
          : `${e.kind} · ${e.text.slice(0, 160)}`}
      </summary>
      <pre>{e.text}</pre>
      {e.payload.length > 0 && <pre>{detail(e)}</pre>}
    </details>
  );
}
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
      <details>
        <summary>Request details</summary>
        <pre>{detail(e)}</pre>
      </details>
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
        <button
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
        </button>
        <button disabled={busy} onClick={() => reply(e, false)}>
          Deny
        </button>
      </div>
    </section>
  );
}
