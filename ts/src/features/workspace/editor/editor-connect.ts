import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

export async function connectWorkspaceEditor(
  c: Connection,
  id: string,
  signal: AbortSignal,
): Promise<{ workspace: string; simulated: boolean }> {
  if (c.inProcess)
    return c.projects.editor({ ref: ref(id) }, { signal, timeoutMs: 180000 });
  const response = await fetch(new URL("/editor/connect", c.baseUrl), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ project: id }),
    signal: AbortSignal.any([signal, AbortSignal.timeout(180000)]),
  });
  if (!response.ok) throw new Error(await response.text());
  const result = await response.json();
  if (typeof result.workspace !== "string")
    throw new Error("Invalid editor connection response");
  return { workspace: result.workspace, simulated: false };
}
