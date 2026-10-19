import { Code, ConnectError } from "@connectrpc/connect";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import type { SessionControlConnection } from "#src/features/session/model/session-control-batch.ts";
export type DashboardConnection = SessionControlConnection &
  Pick<Connection, "projects" | "accounts" | "taskCatalog">;

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
