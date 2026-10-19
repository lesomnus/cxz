import { Code, ConnectError } from "@connectrpc/connect";
import type { Project } from "#gen/cxz/project_pb";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import { sessionInfo } from "#src/features/session/model/session-info.ts";
import { payload } from "#src/features/session/model/journal.ts";
import { isDiagnosticEvent } from "#src/features/session/model/event-diagnostic.ts";

export type DashboardConnection = Pick<
  Connection,
  "projects" | "sessions" | "accounts" | "taskCatalog"
>;
export const RECENT_SESSION_ROWS = 32;

export function sessionCounts(sessions: Session[]) {
  return {
    total: sessions.length,
    working: sessions.filter((s) =>
      ["working", "running"].includes(s.status?.state ?? ""),
    ).length,
    waiting: sessions.filter((s) =>
      ["waiting_input", "waiting"].includes(s.status?.state ?? ""),
    ).length,
    idle: sessions.filter((s) => s.status?.state === "idle").length,
    stopped: sessions.filter((s) =>
      ["stopped", "interrupted", "failed"].includes(s.status?.state ?? ""),
    ).length,
    unknown: sessions.filter(
      (s) =>
        ![
          "working",
          "running",
          "waiting_input",
          "waiting",
          "idle",
          "stopped",
          "interrupted",
          "failed",
        ].includes(s.status?.state ?? ""),
    ).length,
  };
}

export function recentSession(
  session: Session,
  events: SessionEvent[],
  metadata: SessionEvent[],
) {
  const ordered = [
    ...new Map([...metadata, ...events].map((e) => [e.seq, e])).values(),
  ].sort((a, b) => (a.seq < b.seq ? -1 : a.seq > b.seq ? 1 : 0));
  const info = sessionInfo(session, ordered);
  // Session.model is the creation setting; only the current run's catalog
  // establishes what is selected now, including after a restart.
  const catalog = [...ordered]
    .reverse()
    .find(
      (e) =>
        e.kind === "models" && (!e.runId || e.runId === session.status?.runId),
    );
  const selectedModel = catalog ? payload(catalog) : {};
  const hasModel = [selectedModel.effective_model, selectedModel.model].some(
    (value) => typeof value === "string" && value.length > 0,
  );
  return {
    info: {
      ...info,
      model: hasModel ? info.model : "",
      effort: hasModel ? info.effort : "",
    },
    activity: [...events]
      .reverse()
      .filter((e) => ["input", "assistant"].includes(e.kind))
      .slice(0, 6),
    errors: [...events].reverse().filter(isDiagnosticEvent).slice(0, 3),
  };
}

function resolvePath(base: string, value: string) {
  const segments: string[] = [];
  for (const part of (value.startsWith("/") ? value : `${base}/${value}`).split(
    "/",
  )) {
    if (part === "..") segments.pop();
    else if (part && part !== ".") segments.push(part);
  }
  return `/${segments.join("/")}`;
}

// Read only tiny Git metadata files, never start a shell or download a repository.
export async function projectBranch(
  c: DashboardConnection,
  project: Project,
  signal: AbortSignal,
) {
  if (project.status?.state !== "running") return { state: "offline" as const };
  const workspace = project.status?.remoteWorkspace;
  if (!workspace) return { state: "unavailable" as const };
  async function read(path: string) {
    let text = "",
      size = 0;
    const decoder = new TextDecoder();
    for await (const reply of c.projects.download(
      { ref: ref(project.runtimeId), path },
      { signal, timeoutMs: 10000 },
    )) {
      size += reply.data.byteLength;
      if (reply.totalSize > 4096n || size > 4096)
        throw new ConnectError(
          "Git metadata exceeds the read limit",
          Code.ResourceExhausted,
        );
      text += decoder.decode(reply.data, { stream: true });
    }
    signal.throwIfAborted();
    return (text + decoder.decode()).trim();
  }
  const directory = resolvePath(workspace, ".git");
  let head: string;
  try {
    head = await read(`${directory}/HEAD`);
  } catch (error) {
    signal.throwIfAborted();
    if (!(error instanceof ConnectError)) throw error;
    const code = ConnectError.from(error).code;
    if (![Code.NotFound, Code.InvalidArgument, Code.Unknown].includes(code))
      throw error;
    // A linked worktree/submodule uses a .git pointer instead of a directory.
    try {
      const pointer = await read(directory);
      if (!pointer.startsWith("gitdir: "))
        return { state: "unavailable" as const };
      head = await read(
        `${resolvePath(workspace, pointer.slice(8).trim())}/HEAD`,
      );
    } catch (pointerError) {
      signal.throwIfAborted();
      if (ConnectError.from(pointerError).code === Code.NotFound)
        return { state: "missing" as const };
      throw pointerError;
    }
  }
  if (head.startsWith("ref: refs/heads/"))
    return { state: "branch" as const, value: head.slice(16) };
  if (/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/i.test(head))
    return { state: "detached" as const, value: head.slice(0, 8) };
  return { state: "unavailable" as const };
}
