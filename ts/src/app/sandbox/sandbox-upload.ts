import {
  accepted,
  type Transfer,
} from "#src/features/session/composer/composer-upload.ts";

/** The field the worker's Go half reads, and the field it answers on. */
const Message = "cxz-attachment";
const MessageResult = "cxz-attachment-result";

interface Worker {
  postMessage(message: unknown, transfer?: unknown[]): void;
  addEventListener(type: string, fn: (ev: unknown) => void): void;
  removeEventListener(type: string, fn: (ev: unknown) => void): void;
}

/**
 * sandboxTransfer carries a file body to the server running in the page.
 *
 * There is no socket to POST to -- the server is a Go instance in a Worker --
 * so the body travels over the message channel the calls already share, and
 * the Go half turns it into the same `*http.Request` the Manager's listener
 * would have received. The reply is that handler's, unaltered, which is why
 * [accepted] reads it the same way it reads an XHR's.
 *
 * The buffer is transferred rather than copied: a page that drops a 200MB file
 * on the sandbox should not hold it twice while the worker reads it.
 */
export function sandboxTransfer(worker: Worker): Transfer {
  let sequence = 0;
  return async (url, file, signal, onProgress) => {
    signal.throwIfAborted();
    const body = await file.arrayBuffer();
    signal.throwIfAborted();
    const id = `upload-${++sequence}`;
    return new Promise<string>((resolve, reject) => {
      const done = () => {
        worker.removeEventListener("message", received);
        signal.removeEventListener("abort", aborted);
      };
      const received = (ev: unknown) => {
        const data = (ev as MessageEvent).data;
        if (data?.cxz !== MessageResult || data.id !== id) return;
        done();
        // Progress is the transfer's, and this one has no frames to report: the
        // body is handed over whole. Saying so once keeps a caller's progress
        // from stalling at zero on a reply that already arrived.
        onProgress?.(file.size, file.size);
        try {
          resolve(accepted(data.status, "application/json", data.body));
        } catch (error) {
          reject(error);
        }
      };
      const aborted = () => {
        done();
        reject(signal.reason ?? new DOMException("Aborted", "AbortError"));
      };
      worker.addEventListener("message", received);
      signal.addEventListener("abort", aborted, { once: true });
      worker.postMessage(
        { cxz: Message, id, url: url.pathname + url.search, body },
        [body],
      );
    });
  };
}
