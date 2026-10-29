import type { UploadProgress } from "#src/features/session/composer/composer-upload.ts";

// Connect still owns serialization, credentials, errors and RPC interceptors.
// Only its unary HTTP exchange uses XHR so upload progress is observable.
export function uploadFetch(onProgress: UploadProgress): typeof fetch {
  return async (input, init) => {
    const request = new Request(input, { ...init, credentials: "same-origin" });
    const body = await request.arrayBuffer();
    if (request.signal.aborted) {
      new Uint8Array(body).fill(0);
      request.signal.throwIfAborted();
    }
    return new Promise<Response>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const cleanup = () => {
        request.signal.removeEventListener("abort", abort);
        xhr.upload.onprogress = null;
        xhr.onload = xhr.onerror = xhr.onabort = xhr.ontimeout = null;
        new Uint8Array(body).fill(0);
      };
      const fail = (error: unknown) => {
        cleanup();
        reject(error);
      };
      const abort = () => {
        fail(
          request.signal.reason ?? new DOMException("Aborted", "AbortError"),
        );
        xhr.abort();
      };
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) onProgress(event.loaded, event.total);
      };
      xhr.onload = () => {
        try {
          const headers = new Headers();
          for (const line of xhr
            .getAllResponseHeaders()
            .trim()
            .split(/[\r\n]+/)) {
            const colon = line.indexOf(":");
            if (colon > 0)
              headers.append(
                line.slice(0, colon),
                line.slice(colon + 1).trim(),
              );
          }
          const response = new Response(
            [204, 205, 304].includes(xhr.status) ? null : xhr.response,
            { status: xhr.status, statusText: xhr.statusText, headers },
          );
          cleanup();
          resolve(response);
        } catch (error) {
          fail(error);
        }
      };
      xhr.onerror = () => fail(new TypeError("Secret upload failed."));
      xhr.onabort = () =>
        fail(
          request.signal.reason ?? new DOMException("Aborted", "AbortError"),
        );
      xhr.ontimeout = () =>
        fail(new DOMException("Upload timed out.", "TimeoutError"));
      request.signal.addEventListener("abort", abort, { once: true });
      try {
        request.signal.throwIfAborted();
        xhr.open(request.method, request.url);
        xhr.responseType = "arraybuffer";
        for (const [name, value] of request.headers)
          xhr.setRequestHeader(name, value);
        onProgress(0, body.byteLength);
        xhr.send(body);
      } catch (error) {
        fail(error);
      }
    });
  };
}
