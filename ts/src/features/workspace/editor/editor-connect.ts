import { projectRef } from "#src/shared/api/project-id.ts";
import type { Connection } from "#src/shared/api/connection.ts";

export async function connectWorkspaceEditor(
  c: Connection,
  id: string,
  signal: AbortSignal,
): Promise<{ workspace: string; simulated: boolean }> {
  if (c.inProcess)
    return c.projects.editor(
      { ref: projectRef(id) },
      { signal, timeoutMs: 180000 },
    );
  const errorKey = JSON.stringify([c.baseUrl, "editor/connect", id]);
  let status: number | undefined;
  try {
    const response = await fetch(new URL("/editor/connect", c.baseUrl), {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: id }),
      signal: AbortSignal.any([signal, AbortSignal.timeout(180000)]),
    });
    status = response.status;
    if (!response.ok) throw new Error(await response.text());
    const result = await response.json();
    if (typeof result.workspace !== "string")
      throw new Error("Invalid editor connection response");
    if (!signal.aborted) c.errors?.resolve(errorKey);
    return { workspace: result.workspace, simulated: false };
  } catch (error) {
    if (!signal.aborted)
      c.errors?.report(errorKey, error, {
        source: "http",
        origin: {
          kind: "http",
          method: "POST",
          path: "/editor/connect",
          status,
        },
        subject: { kind: "project", id },
        connection: c.baseUrl,
        method: "POST /editor/connect",
        project: id,
      });
    throw error;
  }
}
