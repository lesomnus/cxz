import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Project } from "#gen/cxz/project_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import { running } from "./project-state.ts";
export type DashboardConnection = Connection;

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
  if (!running(project)) return { state: "offline" as const };
  const workspace = project.status?.remoteWorkspace;
  if (!workspace) return { state: "unavailable" as const };
  async function read(path: string) {
    let text = "",
      size = 0;
    const decoder = new TextDecoder();
    for await (const reply of c.projects.download(
      { ref: projectRef(projectId(project)), path },
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
