import type { Connection } from "#src/shared/api/connection.ts";

export type UploadProgress = (loaded: number, total: number) => void;
export type UploadFile = (
  file: File,
  signal: AbortSignal,
  onProgress?: UploadProgress,
) => Promise<string>;
export const MAX_UPLOAD_BYTES = 1024 * 1024 * 1024;

/**
 * Transfer is the one thing an upload needs that differs by where the server
 * is: a request with a body, and the answer to it. Every caller above this
 * builds the same request and reads the same reply.
 *
 * The sandbox has one because a page holding its own server has no socket to
 * POST to -- not because its uploads work differently. It carries the body to
 * the same handler and returns what that handler said.
 */
export type Transfer = (
  url: URL,
  file: File,
  signal: AbortSignal,
  onProgress?: UploadProgress,
) => Promise<string>;

export async function uploadFile(
  c: Connection,
  sessionId: string,
  runId: string,
  file: File,
  signal: AbortSignal,
  onProgress?: UploadProgress,
) {
  signal.throwIfAborted();
  if (file.size > MAX_UPLOAD_BYTES)
    throw new Error("Files are limited to 1 GiB.");
  const url = new URL(
    `/attachments/${encodeURIComponent(sessionId)}`,
    c.baseUrl,
  );
  url.search = new URLSearchParams({
    run: runId,
    name: file.name,
    size: String(file.size),
  }).toString();
  const path = await (c.transfer ?? uploadHttp)(url, file, signal, onProgress);
  if (typeof path !== "string" || !path.startsWith("/"))
    throw new Error("Upload completed without a file path.");
  return path;
}

/**
 * accepted reads one upload's reply, whichever transfer carried it. The reply
 * is the handler's, so it is the same shape in the sandbox as on a socket: a
 * JSON path on success, and on failure a JSON message or whatever text the
 * status came with.
 */
export function accepted(
  status: number,
  contentType: string | null,
  text: string,
): string {
  let body: { path?: string; message?: string } | undefined;
  try {
    body = JSON.parse(text);
  } catch {
    // Non-JSON failures still use the HTTP response's message below.
  }
  if (status < 200 || status >= 300) {
    const message = contentType?.includes("application/json")
      ? body?.message
      : text.trim();
    throw new Error(
      typeof message === "string" && message ? message : "Upload failed.",
    );
  }
  // The caller validates the acknowledgement, including missing paths.
  return body?.path as string;
}

// Keep the existing raw HTTP body and same-origin cookies. XHR exposes actual
// bytes transferred without reading the entire Blob into JavaScript memory.
// Transfer progress is separate from the server's final path acknowledgement.
function uploadHttp(
  url: URL,
  file: File,
  signal: AbortSignal,
  onProgress?: UploadProgress,
): Promise<string> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted();
    const request = new XMLHttpRequest();
    const cleanup = () => {
      signal.removeEventListener("abort", abort);
      request.upload.onprogress = null;
      request.onload =
        request.onerror =
        request.onabort =
        request.ontimeout =
          null;
    };
    const fail = (error: unknown) => {
      cleanup();
      reject(error);
    };
    const abort = () => {
      fail(signal.reason ?? new DOMException("Aborted", "AbortError"));
      request.abort();
    };
    request.upload.onprogress = (event) => {
      if (event.lengthComputable) onProgress?.(event.loaded, event.total);
    };
    request.onload = () => {
      let path: string;
      try {
        path = accepted(
          request.status,
          request.getResponseHeader("Content-Type"),
          request.responseText,
        );
      } catch (error) {
        fail(error);
        return;
      }
      cleanup();
      resolve(path);
    };
    request.onerror = () => fail(new Error("Upload failed."));
    request.onabort = () =>
      fail(signal.reason ?? new DOMException("Aborted", "AbortError"));
    request.ontimeout = () =>
      fail(new DOMException("Upload timed out.", "TimeoutError"));
    signal.addEventListener("abort", abort, { once: true });
    try {
      request.open("POST", url);
      request.timeout = 600_000;
      request.setRequestHeader("Content-Type", "application/octet-stream");
      request.send(file);
    } catch (error) {
      fail(error);
    }
  });
}
