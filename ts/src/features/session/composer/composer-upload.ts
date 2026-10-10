import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

export type UploadFile = (file: File, signal: AbortSignal) => Promise<string>;
export const MAX_UPLOAD_BYTES = 1024 * 1024 * 1024;

export async function uploadFile(
  c: Connection,
  sessionId: string,
  runId: string,
  file: File,
  signal: AbortSignal,
) {
  if (file.size > MAX_UPLOAD_BYTES)
    throw new Error("Files are limited to 1 GiB.");
  let path: string;
  if (c.inProcess) {
    async function* messages() {
      yield {
        ref: ref(sessionId),
        runId,
        name: file.name,
        size: BigInt(file.size),
      };
      for (let offset = 0; offset < file.size; offset += 256 * 1024) {
        signal.throwIfAborted();
        yield {
          content: new Uint8Array(
            await file.slice(offset, offset + 256 * 1024).arrayBuffer(),
          ),
        };
      }
    }
    path = (await c.sessions.upload(messages(), { signal, timeoutMs: 600_000 }))
      .path;
  } else {
    const url = new URL(
      `/attachments/${encodeURIComponent(sessionId)}`,
      c.baseUrl,
    );
    url.search = new URLSearchParams({
      run: runId,
      name: file.name,
      size: String(file.size),
    }).toString();
    const response = await fetch(url, {
      method: "POST",
      credentials: "same-origin",
      signal,
      headers: { "Content-Type": "application/octet-stream" },
      body: file,
    });
    if (!response.ok)
      throw new Error((await response.text()).trim() || "Upload failed.");
    path = (await response.json()).path;
  }
  if (typeof path !== "string" || !path.startsWith("/"))
    throw new Error("Upload completed without a file path.");
  return path;
}
