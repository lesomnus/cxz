import { createDrpcTransport } from "@lesomnus/grpc-dgram/transport/connect";
import type { Sandbox } from "@lesomnus/payday/sandbox";
import type { Transport } from "@connectrpc/connect";

export function sandboxTransport({ sock, transport }: Sandbox): Transport {
  // grpc-dgram 0.0.1 reserves a stream ID before Connect awaits its input.
  // A concurrent unary call can open a higher ID first, making the reliable
  // server reject this stream as stale. Give each stream its own MessagePort
  // peer so opening requests cannot overtake each other on the same peer.
  return {
    ...transport,
    async stream(...args) {
      const conn = sock.dial();
      const signal = args[1];
      const close = () => {
        signal?.removeEventListener("abort", close);
        conn.close();
      };
      signal?.addEventListener("abort", close, { once: true });
      if (signal?.aborted) close();
      try {
        const response = await createDrpcTransport(conn).stream(...args);
        return {
          ...response,
          message: (async function* () {
            try {
              yield* response.message;
            } finally {
              close();
            }
          })(),
        };
      } catch (error) {
        close();
        throw error;
      }
    },
  };
}
