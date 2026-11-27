import { Id } from "@lesomnus/payday/pdid";
import { createContext, useContext, useMemo } from "react";
import { standaloneConnection } from "#src/shared/api/routable-connections.ts";

export const ConnectionScope = createContext(standaloneConnection);

// Sequence numbers can exceed JavaScript's safe integer range.
export function parseRouteSearch(search: string) {
  const values = Object.fromEntries(new URLSearchParams(search));
  // Older links JSON-quoted numeric strings to preserve their type.
  if (/^"[1-9]\d*"$/.test(values.seq ?? ""))
    values.seq = values.seq.slice(1, -1);
  return values;
}
export function stringifyRouteSearch(search: Record<string, unknown>) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(search))
    if (value !== undefined && value !== null) params.set(key, String(value));
  const result = params.toString();
  return result ? `?${result}` : "";
}

export function uuid(value: string): Uint8Array | undefined {
  if (!/^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i.test(value))
    return;
  const hex = value.replaceAll("-", "");
  return Uint8Array.from({ length: 16 }, (_, i) =>
    Number.parseInt(hex.slice(2 * i, 2 * i + 2), 16),
  );
}

export function resourceRef(value: string) {
  const id = uuid(value);
  return {
    key: id
      ? { case: "id" as const, value: id }
      : { case: "alias" as const, value },
  };
}

export function resourceHandle(resource: {
  id: Uint8Array;
  alias?: string;
  runtimeId?: string;
}) {
  return (
    resource.alias ||
    (resource.id.length === 16
      ? new Id(resource.id).toString()
      : resource.runtimeId || "")
  );
}

export function matchesResource(
  resource: { id: Uint8Array; alias?: string; runtimeId?: string },
  handle: string,
) {
  if (!handle) return false;
  const id = uuid(handle);
  if (id)
    return (
      resource.id.length === id.length &&
      resource.id.every((v, i) => v === id[i])
    );
  return resource.alias === handle || resource.runtimeId === handle;
}

export const resourcePaths = {
  sessions: "/c/$connection/session",
  projects: "/c/$connection/project",
  tasks: "/c/$connection/task",
  accounts: "/c/$connection/account",
  search: "/c/$connection/search",
  errors: "/c/$connection/errors",
} as const;

export function resourceLocation(
  resource: keyof typeof resourcePaths,
  connection = standaloneConnection,
) {
  return { to: resourcePaths[resource], params: { connection } };
}
export function sessionLocation(
  session: string | Parameters<typeof resourceHandle>[0],
  connection = standaloneConnection,
) {
  return {
    to: "/c/$connection/session/$sessionId" as const,
    params: {
      connection,
      sessionId:
        typeof session === "string" ? session : resourceHandle(session),
    },
  };
}
export function projectLocation(
  project?: string,
  connection = standaloneConnection,
) {
  return project
    ? {
        to: "/c/$connection/project/$projectId" as const,
        params: { connection, projectId: project },
      }
    : resourceLocation("projects", connection);
}
export function memoryLocation(
  project: string | Parameters<typeof resourceHandle>[0],
  memory?: string,
  connection = standaloneConnection,
) {
  const projectId =
    typeof project === "string" ? project : resourceHandle(project);
  return memory
    ? {
        to: "/c/$connection/project/$projectId/memory/$memoryId" as const,
        params: { connection, projectId, memoryId: memory },
      }
    : {
        to: "/c/$connection/project/$projectId/memory" as const,
        params: { connection, projectId },
      };
}
export function accountLocation(
  account?: string,
  connection = standaloneConnection,
) {
  return account
    ? {
        to: "/c/$connection/account/$accountId" as const,
        params: { connection, accountId: account },
      }
    : resourceLocation("accounts", connection);
}
export function taskLocation(
  {
    task,
    project,
    ...search
  }: {
    task?: string | Parameters<typeof resourceHandle>[0];
    project?: string | Parameters<typeof resourceHandle>[0];
    assignee?: string;
    run?: string;
  },
  connection = standaloneConnection,
) {
  const projectId =
    typeof project === "string" ? project : project && resourceHandle(project);
  const taskId = typeof task === "string" ? task : task && resourceHandle(task);
  if (projectId) {
    return taskId
      ? {
          to: "/c/$connection/project/$projectId/task/$taskId" as const,
          params: { connection, projectId, taskId },
          search,
        }
      : {
          to: "/c/$connection/project/$projectId/task" as const,
          params: { connection, projectId },
          search,
        };
  }
  return taskId
    ? {
        to: "/c/$connection/task/$taskId" as const,
        params: { connection, taskId },
        search,
      }
    : { ...resourceLocation("tasks", connection), search };
}

export function useResourceRoutes() {
  const connection = useContext(ConnectionScope);
  return useMemo(
    () => ({
      resourceLocation: (resource: keyof typeof resourcePaths) =>
        resourceLocation(resource, connection),
      sessionLocation: (session: Parameters<typeof sessionLocation>[0]) =>
        sessionLocation(session, connection),
      projectLocation: (project?: string, target = connection) =>
        projectLocation(project, target),
      memoryLocation: (
        project: Parameters<typeof memoryLocation>[0],
        memory?: string,
      ) => memoryLocation(project, memory, connection),
      accountLocation: (account?: string) =>
        accountLocation(account, connection),
      taskLocation: (task: Parameters<typeof taskLocation>[0]) =>
        taskLocation(task, connection),
    }),
    [connection],
  );
}
