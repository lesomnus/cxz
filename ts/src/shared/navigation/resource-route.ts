import { Id } from "@lesomnus/payday/pdid";

export const selfConnection = { connection: "_" } as const;

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
  memory: "/c/$connection/memory",
  activity: "/c/$connection/activity",
  auxiliary: "/c/$connection/auxiliary",
} as const;

export function resourceLocation(resource: keyof typeof resourcePaths) {
  return { to: resourcePaths[resource], params: selfConnection };
}
export function sessionLocation(
  session: string | Parameters<typeof resourceHandle>[0],
) {
  return {
    to: "/c/$connection/session/$sessionId" as const,
    params: {
      ...selfConnection,
      sessionId:
        typeof session === "string" ? session : resourceHandle(session),
    },
  };
}
export function projectLocation(project?: string) {
  return project
    ? {
        to: "/c/$connection/project/$projectId" as const,
        params: { ...selfConnection, projectId: project },
      }
    : resourceLocation("projects");
}
export function accountLocation(account?: string) {
  return account
    ? {
        to: "/c/$connection/account/$accountId" as const,
        params: { ...selfConnection, accountId: account },
      }
    : resourceLocation("accounts");
}
export function taskLocation({
  task,
  ...search
}: {
  task?: string;
  project?: string;
  assignee?: string;
  run?: string;
}) {
  return task
    ? {
        to: "/c/$connection/task/$taskId" as const,
        params: { ...selfConnection, taskId: task },
        search,
      }
    : { ...resourceLocation("tasks"), search };
}
