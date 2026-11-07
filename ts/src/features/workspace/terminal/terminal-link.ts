import { projectRef } from "#src/shared/api/project-id.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import type { MessageInitShape } from "@bufbuild/protobuf";
import type { ProjectTerminalRequestSchema } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import { createTerminalAcknowledger } from "./terminal-acknowledger.ts";
import { ConnectError } from "@connectrpc/connect";
import { clientErrors } from "#src/shared/errors/client-errors.ts";

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
  const errors = c.errors ?? clientErrors();
  const errorKey = JSON.stringify([c.baseUrl, "terminal", project]);
  const errorContext = {
    source: "resource" as const,
    origin: {
      kind: "connection",
      purpose: "terminal",
      transport: c.inProcess ? "rpc" : "websocket",
    } as const,
    subject: { kind: "project", id: project } as const,
    connection: c.baseUrl,
    method: "Project terminal",
    project,
  };
  const deliverStatus = (frame: TerminalStatus) => {
    if (frame.error)
      errors.report(errorKey, new Error(frame.error), errorContext);
    else if (frame.ready) errors.resolve(errorKey);
    status(frame);
  };
  if (c.inProcess) {
    const abort = new AbortController();
    const queue = new InputQueue();
    queue.push({ ref: projectRef(project), columns, rows });
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
          if (frame.ready || frame.exited || frame.error) deliverStatus(frame);
        }
      } catch (error) {
        if (!abort.signal.aborted)
          (error instanceof ConnectError ? status : deliverStatus)({
            error: String(error),
          });
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
  const acknowledger = createTerminalAcknowledger((ack) => {
    // Input congestion must not prevent output credit from reaching the server.
    if (ws.readyState === WebSocket.OPEN) write(JSON.stringify({ ack }));
  });
  const write = (data: string | Uint8Array) => {
    try {
      ws.send(data);
    } catch (error) {
      closed = true;
      deliverStatus({ error: String(error) });
      ws.close();
    }
  };
  const send = (data: string | Uint8Array) => {
    if (ws.readyState !== WebSocket.OPEN) return;
    if (ws.bufferedAmount > 1 << 20) {
      closed = true;
      deliverStatus({
        error: t("Terminal input is congested; reconnect to try again"),
      });
      ws.close();
      return;
    }
    write(data);
  };
  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      const bytes = new Uint8Array(event.data);
      let consumed = false;
      output(bytes, () => {
        if (consumed) return;
        consumed = true;
        acknowledger.parsed(bytes.length);
      });
    } else {
      let frame: TerminalStatus;
      try {
        frame = JSON.parse(event.data);
      } catch {
        closed = true;
        deliverStatus({ error: t("Invalid terminal response") });
        ws.close();
        return;
      }
      if (frame.exited || frame.error) closed = true;
      deliverStatus(frame);
    }
  };
  ws.onclose = (event) => {
    acknowledger.dispose();
    if (!closed) {
      const message = t("Terminal disconnected; reconnect to try again");
      errors.report(errorKey, new Error(message), {
        ...errorContext,
        code: String(event.code),
        origin: { ...errorContext.origin, closeCode: event.code },
      });
      status({ error: message });
    }
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
      acknowledger.dispose();
      ws.close();
    },
  };
}
