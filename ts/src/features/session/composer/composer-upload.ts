import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

export type UploadProgress = (loaded: number, total: number) => void;
export type UploadFile = (
  file: File,
  signal: AbortSignal,
  onProgress?: UploadProgress,
) => Promise<string>;
export const MAX_UPLOAD_BYTES = 1024 * 1024 * 1024;

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
        const content = new Uint8Array(
          await file.slice(offset, offset + 256 * 1024).arrayBuffer(),
        );
        yield {
          content,
        };
        onProgress?.(offset + content.length, file.size);
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
    path = await uploadHttp(url, file, signal, onProgress);
  }
  if (typeof path !== "string" || !path.startsWith("/"))
    throw new Error("Upload completed without a file path.");
  return path;
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
      let response: { path?: string; message?: string } | undefined;
      try {
        response = JSON.parse(request.responseText);
      } catch {
        // Non-JSON failures still use the HTTP response's message below.
      }
      if (request.status < 200 || request.status >= 300) {
        const message = request
          .getResponseHeader("Content-Type")
          ?.includes("application/json")
          ? response?.message
          : request.responseText.trim();
        fail(
          new Error(
            typeof message === "string" && message ? message : "Upload failed.",
          ),
        );
        return;
      }
      cleanup();
      // The caller validates the acknowledgement, including missing paths.
      resolve(response?.path as string);
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
