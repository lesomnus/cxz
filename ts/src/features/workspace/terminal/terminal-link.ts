import { t } from "#src/shared/i18n/i18n.ts";
import type { MessageInitShape } from "@bufbuild/protobuf";
import type { ProjectTerminalRequestSchema } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

type Frame = MessageInitShape<typeof ProjectTerminalRequestSchema>;
export type TerminalStatus = {
  ready?: boolean;
  exited?: boolean;
  error?: string;
};
export type TerminalLink = {
  input: (text: string) => void;
  resize: (columns: number, rows: number) => void;
  close: () => void;
};

// A bounded producer for the WASM transport's bidirectional gRPC stream.
class InputQueue {
  frames: Frame[] = [];
  wake?: () => void;
  ended = false;
  push(frame: Frame) {
    if (this.ended) return;
    if (this.frames.length >= 128)
      throw new Error(t("Terminal input queue is full"));
    this.frames.push(frame);
    this.wake?.();
  }
  close() {
    this.ended = true;
    this.frames = [];
    this.wake?.();
  }
  async *read() {
    while (!this.ended) {
      if (!this.frames.length)
        await new Promise<void>((resolve) => {
          this.wake = resolve;
        });
      this.wake = undefined;
      const frame = this.frames.shift();
      if (frame) yield frame;
    }
  }
}

export function openTerminal(
  c: Connection,
  project: string,
  columns: number,
  rows: number,
  output: (bytes: Uint8Array, consumed: () => void) => void,
  status: (status: TerminalStatus) => void,
): TerminalLink {
  const encoder = new TextEncoder();
  if (c.inProcess) {
    const abort = new AbortController();
    const queue = new InputQueue();
    queue.push({ ref: ref(project), columns, rows });
    void (async () => {
      try {
        for await (const frame of c.projects.terminal(queue.read(), {
          signal: abort.signal,
        })) {
          if (frame.output?.length)
            await new Promise<void>((resolve) => {
              const consumed = () => {
                abort.signal.removeEventListener("abort", consumed);
                resolve();
              };
              if (abort.signal.aborted) {
                resolve();
                return;
              }
              abort.signal.addEventListener("abort", consumed, { once: true });
              output(frame.output!, consumed);
            });
          if (frame.ready || frame.exited || frame.error) status(frame);
        }
      } catch (error) {
        if (!abort.signal.aborted) status({ error: String(error) });
      } finally {
        queue.close();
      }
    })();
    return {
      input: (text) => {
        const bytes = encoder.encode(text);
        for (let i = 0; i < bytes.length; i += 32768)
          queue.push({ input: bytes.slice(i, i + 32768) });
      },
      resize: (columns, rows) => queue.push({ columns, rows }),
      close: () => {
        queue.close();
        abort.abort();
      },
    };
  }
  const url = new URL(`/terminal/${encodeURIComponent(project)}`, c.baseUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.search = new URLSearchParams({
    columns: String(columns),
    rows: String(rows),
  }).toString();
  const ws = new WebSocket(url);
  ws.binaryType = "arraybuffer";
  let closed = false;
  const send = (data: string | Uint8Array) => {
    if (ws.readyState !== WebSocket.OPEN) return;
    if (ws.bufferedAmount > 1 << 20) {
      status({
        error: t("Terminal input is congested; reconnect to try again"),
      });
      ws.close();
      return;
    }
    ws.send(data);
  };
  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      const bytes = new Uint8Array(event.data);
      output(bytes, () => send(JSON.stringify({ ack: bytes.length })));
    } else {
      const frame: TerminalStatus = JSON.parse(event.data);
      if (frame.exited || frame.error) closed = true;
      status(frame);
    }
  };
  ws.onclose = () => {
    if (!closed)
      status({ error: t("Terminal disconnected; reconnect to try again") });
  };
  return {
    input: (text) => {
      const bytes = encoder.encode(text);
      for (let i = 0; i < bytes.length; i += 32768)
        send(bytes.slice(i, i + 32768));
    },
    resize: (columns, rows) => send(JSON.stringify({ columns, rows })),
    close: () => {
      closed = true;
      ws.close();
    },
  };
}
