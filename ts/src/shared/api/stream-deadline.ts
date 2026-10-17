import { Code, ConnectError } from "@connectrpc/connect";

/**
 * A stream that stays open and says nothing is the failure neither end
 * notices: the reader is waiting, the writer thinks it has nothing to write,
 * and nothing ends or throws. Reading it with a deadline is what turns that
 * silence into an error, which is what every caller here already knows how to
 * handle -- it reconnects. See cxz#188 and cxz#189.
 *
 * The abandoned read stays attached to the race, so its own later rejection is
 * already handled. Dropping its value costs nothing: the only reason to be
 * here is that there was no value.
 */
export function readBy<T>(
  read: Promise<T>,
  ms: number,
  what: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  return Promise.race([
    read,
    new Promise<never>((_, reject) => {
      timer = setTimeout(
        () => reject(new ConnectError(what, Code.Unavailable)),
        ms,
      );
    }),
  ]).finally(() => clearTimeout(timer));
}

/**
 * Releasing an iterator is best effort and deliberately not awaited: one
 * suspended on a read that never completes -- the case a deadline reports --
 * would never process the return, and awaiting it would swallow the error
 * being thrown. The request itself goes when its signal is aborted.
 */
export function release(stream: { return?: () => Promise<unknown> }) {
  void stream.return?.().catch(() => {});
}

/**
 * How long a heartbeated stream may say nothing before it is reported. Two
 * missed beats of the runtime's own interval, and it lives here rather than
 * beside either reader: both streams of a conversation are heartbeated by the
 * same runtime, so a reader that chose its own number would be guessing.
 */
export const STREAM_STALL_MS = 30_000;

/**
 * alive reads a stream that says so. A sender heartbeats when it has nothing
 * to send, because only it knows whether silence means idle or stalled; this
 * yields the real frames, drops the heartbeats, and throws once two of them
 * are missed.
 *
 * The deadline is armed by the first heartbeat rather than by opening the
 * stream. A sender that predates them is legitimately silent, and timing it
 * out would reconnect a healthy stream forever.
 */
export async function* alive<T>(
  source: AsyncIterable<T>,
  beat: (value: T) => boolean,
  stall: number,
  what: string,
): AsyncGenerator<T> {
  const stream = source[Symbol.asyncIterator]();
  let armed = false;
  try {
    for (;;) {
      const next = await (armed
        ? readBy(stream.next(), stall, what)
        : stream.next());
      if (next.done) return;
      if (beat(next.value)) {
        armed = true;
        continue;
      }
      yield next.value;
    }
  } finally {
    release(stream);
  }
}
